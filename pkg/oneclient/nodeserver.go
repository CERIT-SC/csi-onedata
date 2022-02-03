package oneclient

import (
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"

	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounts map[string]*mountPoint
}

type mountPoint struct {
//	Host      string
	VolumeId  string
	MountPath string
	Token     string
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	notMnt, e := mount.New("").IsLikelyNotMountPoint(targetPath)
	if e != nil {
		if os.IsNotExist(e) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	token := "token_init"
	host := req.GetVolumeCapability().GetMount().GetFsType()
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	/*
	if mountOptions == nil {
		token = "JE TO NULL"
	}
	if len(mountOptions) == 0 {
		token = "JE TO NULA"
	}
        if req.GetVolumeCapability().GetMount() == nil {
		token = "NULL vsude"
	}*/
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro", "NECO")
	}
	if e := validateVolumeContext(req); e != nil {
		return nil, e
	}

	/*
	sshOpts := req.GetVolumeContext()["sshOpts"]
	*/

//	host := req.GetVolumeContext()["host"]
//	token := req.GetVolumeContext()["token"]

	/*
	secret, e := getPublicKeySecret(privateKey)
	if e != nil {
		return nil, e
	}
	privateKeyPath, e := writePrivateKey(secret)
	if e != nil {
		return nil, e
	}*/

	e = Mount(host, targetPath, token, mountOptions)
	if e != nil {
		if os.IsPermission(e) {
			return nil, status.Error(codes.PermissionDenied, e.Error())
		}
		if strings.Contains(e.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		return nil, status.Error(codes.Internal, e.Error())
	}
	ns.mounts[req.VolumeId] = &mountPoint{Token: token, MountPath: targetPath, VolumeId: req.VolumeId}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}

	err = util.UnmountPath(req.GetTargetPath(), mount.New(""))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if point, ok := ns.mounts[req.VolumeId]; ok {
		/*
		err := os.Remove(point.Token)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		*/
		delete(ns.mounts, point.VolumeId)
		glog.Infof("successfully unmount volume: %s", point)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func validateVolumeContext(req *csi.NodePublishVolumeRequest) error {
	/*
	if _, ok := req.GetVolumeContext()["host"]; !ok {
		return status.Errorf(codes.InvalidArgument, "missing volume context value: host")
	}
	if _, ok := req.GetVolumeContext()["token"]; !ok {
		return status.Errorf(codes.InvalidArgument, "missing volume context value: token")
	}
	*/
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

func writePrivateKey(secret *v1.Secret) (string, error) {
	f, e := ioutil.TempFile("", "pk-*")
	defer f.Close()
	if e != nil {
		return "", status.Errorf(codes.Internal, "can not create tmp file for pk: %s", e)
	}

	_, e = f.Write(secret.Data[v1.SSHAuthPrivateKey])
	if e != nil {
		return "", status.Errorf(codes.Internal, "can not create tmp file for pk: %s", e)
	}
	e = f.Chmod(0600)
	if e != nil {
		return "", status.Errorf(codes.Internal, "can not change rights for pk: %s", e)
	}
	return f.Name(), nil
}

// TODO sshOpts string
func Mount(host string, target string, token string, mountOptions []string) error {
	mountCmd := "mount"
	mountArgs := []string{}

	mountArgs = append(
		mountArgs,
		"-t", "onedata",
		"-o", "allow_other",
		"-o", "onedata_token="+token,
	)

	if len(mountOptions) == 0 {
		mountArgs = append(
			mountArgs,
			"!mountOptions je prazdny!",
		)
	}

	for _, option := range mountOptions {
		mountArgs = append(
			mountArgs,
			"--mo", option,
		)
	}

	mountArgs = append(
		mountArgs,
		host,
		target,
	)

	/*
	if len(sshOpts) > 0 {
		mountArgs = append(mountArgs, "-o", sshOpts)
	}*/

	// create target, os.Mkdirall is noop if it exists
	err := os.MkdirAll(target, 0750)
	if err != nil {
		return err
	}

	glog.Infof("executing mount command cmd=%s, args=%s", mountCmd, mountArgs)

	out, err := exec.Command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}

	return nil
}
