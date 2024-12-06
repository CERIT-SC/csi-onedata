package oneclient

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounts map[string]*mountPoint
}

type mountPoint struct {
	VolumeId         string
	MountPath        string
	Token            string
	SpaceId          string
	OneclientOptions string
}

func DetectMountCorruption(path string) (exists bool, isCorrupted bool, err error) {
	_, statErr := os.Stat(path)
	if statErr == nil {
		// Directory exists and is accessible
		return true, false, nil
	}

	// Check if the error is about corrupted mount point
	if strings.Contains(strings.ToLower(statErr.Error()), "transport endpoint is not connected") {
		return true, true, fmt.Errorf("Directory exists but mountpoint is disconnected for unknown reason (path: %s): %w", path, statErr)
	}

	// Check for specific syscall errors
	if pathErr, ok := statErr.(*os.PathError); ok {
		errno, ok := pathErr.Err.(syscall.Errno)
		if ok {
			switch errno {
			case syscall.EACCES:
				return true, false, fmt.Errorf("Directory exists but stat() failed, likely due to permission error (path: %s): %w", path, statErr)
			case syscall.EIO:
				return true, true, fmt.Errorf("Directory exists but stat() failed, likely due to I/O error of corrupted mount (path: %s): %w", path, statErr)
			}
		}
	}

	if os.IsNotExist(statErr) {
		// List mountpoints and check if the directory is a mountpoint
		mounts, err := mount.New("").List()
		if err != nil {
			return false, false, fmt.Errorf("Failed to list mountpoints while checking mountpoint path (path: %s): %w", path, err)
		}
		for _, mount := range mounts {
			if mount.Path == path {
				return true, true, fmt.Errorf("Directory exists but is a corrupted mountpoint likely due to a problem with oneprovider (path: %s): %w", path, statErr)
			}
		}

		return false, false, fmt.Errorf("Directory not found and not a mountpoint (path: %s): %w", path, statErr)
	}

	return true, false, fmt.Errorf("Failed to check mountpoint path with unknown error while checking it using stat() (path: %s): %w", path, statErr)
}

func UnmountCorrupted(mountPoint string) {
	if err := mount.New("").Unmount(mountPoint); err != nil {
		glog.Errorf("Failed to unmount the corrupted mount \"%s\": %v", mountPoint, err)
	} else {
		glog.Infof("Successfully unmounted the corrupted mount \"%s\"", mountPoint)
	}
}

func RemoveVolumeFromMounts(volumeId string, mounts map[string]*mountPoint) {
	if point, ok := mounts[volumeId]; ok {
		delete(mounts, point.VolumeId)
	}
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.Infof("NodePublishVolume: \"%v\"", req)
	targetPath := req.GetTargetPath()
	notMnt, e := mount.New("").IsNotMountPoint(targetPath)
	if e != nil {
		if os.IsNotExist(e) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				glog.Errorf("Failed to create directory for mountpoint: \"%v\"", err)
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			glog.Errorf("Failed to check mountpoint path: %v", e)
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if !notMnt {
		glog.Warningf("Volume \"%s\" is already mounted", req.VolumeId)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()

	sec := req.GetSecrets()
	token := strings.TrimSuffix(sec["onedata_token"], "\n")
	host := strings.TrimSuffix(sec["host"], "\n")
	spaceId := strings.TrimSuffix(sec["space_id"], "\n")
	oneclientOptions := strings.TrimSuffix(sec["oneclient_options"], "\n")

	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}
	if e := validateVolumeContext(req); e != nil {
		glog.Errorf("Validation of volume context failed: %v", e)
		return nil, e
	}

	// Mount the volume
	e = Mount(host, targetPath, token, spaceId, oneclientOptions, mountOptions)
	if e != nil {
		if os.IsPermission(e) {
			glog.Errorf("Mount failed due to permission error: %v", e)
			return nil, status.Error(codes.PermissionDenied, e.Error())
		}
		if strings.Contains(e.Error(), "invalid argument") {
			glog.Errorf("Mount failed due to invalid argument: %v", e)
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		glog.Errorf("Mount failed for unknown reason: %v", e)
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Test the mount to ensure it was successful. This is designed to catch corrupted mounts, that fails to `stat()`.
	// Unmount the volume if the test fails
	_, _, err := DetectMountCorruption(targetPath)
	if err != nil {
		errMsg := fmt.Sprintf("Mountpoint test failed. Mountpoint seems to be corrupted: mnt: \"%s\", err: %v", targetPath, err)
		glog.Errorf(errMsg)
		// Don't use Unmount from K8s interface. If the mount is corrupted, it will fail.
		// Unmount the mountpoint directly.
		UnmountCorrupted(targetPath)
		return nil, status.Error(codes.Unavailable, errMsg)
	}

	glog.Infof("Successfully mounted \"%s\"", targetPath)
	ns.mounts[req.VolumeId] = &mountPoint{Token: token, SpaceId: spaceId, OneclientOptions: oneclientOptions, MountPath: targetPath, VolumeId: req.VolumeId}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	glog.Infof("NodeUnpublishVolume: %v", req)
	targetPath := req.GetTargetPath()

	// Check if the mountpoint exists, is corrupted or has other issues
	dirExists, dirCorrupted, err := DetectMountCorruption(targetPath)
	if dirCorrupted {
		// Unmount corrupted mount violently without additional checks
		glog.Warningf("Mountpoint marked for unmount seems to be corrupted: mnt: \"%s\", err: %v", targetPath, err)
		glog.Warningf("Unmounting the corrupted mountpoint violently: \"%s\"", targetPath)
		UnmountCorrupted(targetPath)
		RemoveVolumeFromMounts(req.VolumeId, ns.mounts)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}
	if !dirExists {
		// Mountpoint is not found, so unmount is skipped
		glog.Warningf("Mountpoint not found. Nothing to unmount: \"%s\"", targetPath)
		glog.Warningf("Removing volume from list of mounts \"%s\"", targetPath)
		RemoveVolumeFromMounts(req.VolumeId, ns.mounts)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}
	if err != nil {
		// Unmount failed due to another mountpoint issue
		errMsg := fmt.Sprintf("Failed to check mountpoint path using stat(). Unmount skipped. Mnt: \"%s\", err: %v", targetPath, err)
		glog.Errorf(errMsg)
		return nil, status.Error(codes.Internal, errMsg)
	}

	// Use K8s interface to check and unmount the mountpoint
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to check mountpoint path using K8s lib. Unmount skipped. Mnt: \"%s\", err: %v", targetPath, err)
		glog.Errorf(errMsg)
		return nil, status.Error(codes.Internal, errMsg)
	}
	if notMnt {
		errMsg := fmt.Sprintf("Path doesn't seem to be a mountpoint. Unmount skipped. Mnt: \"%s\"", targetPath)
		glog.Warningf(errMsg)
		glog.Warningf("Removing volume from list of mounts \"%s\"", targetPath)
		RemoveVolumeFromMounts(req.VolumeId, ns.mounts)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	// Unmount the volume
	err = util.UnmountPath(req.GetTargetPath(), mount.New(""))
	if err != nil {
		errMsg := fmt.Sprintf("Failed to unmount volume: %v", err)
		glog.Errorf(errMsg)
		return nil, status.Error(codes.Internal, errMsg)
	}

	glog.Infof("Successfully unmounted \"%s\"", targetPath)
	RemoveVolumeFromMounts(req.VolumeId, ns.mounts)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func validateVolumeContext(req *csi.NodePublishVolumeRequest) error {
	sec := req.GetSecrets()
	if sec == nil {
		return status.Errorf(codes.InvalidArgument, "secret is required")
	}
	if _, ok := sec["onedata_token"]; !ok {
		return status.Errorf(codes.InvalidArgument, "\"onedata_token\" is required in secret")
	}
	if _, ok := sec["host"]; !ok {
		return status.Errorf(codes.InvalidArgument, "\"host\" is required in secret")
	}
	if _, ok := sec["space_id"]; !ok {
		return status.Errorf(codes.InvalidArgument, "\"space_id\" is required in secret")
	}
	if _, ok := sec["oneclient_options"]; !ok {
		return status.Errorf(codes.InvalidArgument, "\"oneclient_options\" is required in secret")
	}
	return nil
}

func getPublicKeySecret(secretName string) (*v1.Secret, error) {
	namespaceAndSecret := strings.SplitN(secretName, "/", 2)
	namespace := namespaceAndSecret[0]
	name := namespaceAndSecret[1]

	clientset, e := GetK8sClient()
	if e != nil {
		return nil, status.Errorf(codes.Internal, "can not create kubernetes client: %s", e)
	}

	secret, e := clientset.CoreV1().
		Secrets(namespace).
		Get(name, metav1.GetOptions{})

	if e != nil {
		return nil, status.Errorf(codes.Internal, "can not get secret %s: %s", secretName, e)
	}

	if secret.Type != v1.SecretTypeSSHAuth {
		return nil, status.Errorf(codes.InvalidArgument, "type of secret %s is not %s", secretName, v1.SecretTypeSSHAuth)
	}
	return secret, nil
}

func Mount(host string, mountpointPath string, token string, spaceId string, oneclientOptions string, mountOptions []string) error {
	mountCmd := "mount"
	mountArgs := []string{
		"-t", "onedata",
		"-o", fmt.Sprintf("onedata_token=%s", token),
		"-o", fmt.Sprintf("space_id=%s", spaceId),
		"-o", fmt.Sprintf("oneclient_options=\"%s\"", oneclientOptions),
	}

	mountArgs = append(mountArgs, host, mountpointPath)

	err := os.MkdirAll(mountpointPath, 0750)
	if err != nil {
		return err
	}

	glog.Infof("Executing mount command cmd=%s, args=%s", mountCmd, mountArgs)

	cmd := exec.Command(mountCmd, mountArgs...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		glog.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		glog.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		glog.Fatalf("Failed to start command: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		glog.Errorf("Failed to wait for command: %v", err)
	}
	stdout, _ := io.ReadAll(stdoutPipe)
	stderr, _ := io.ReadAll(stderrPipe)

	if err != nil {
		return fmt.Errorf("Mounting command failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(stdout))
	}

	glog.Infof("Mount command done. stdout: %q, stderr: %q", string(stdout), string(stderr))
	return nil
}
