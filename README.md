# Container Storage Interface Driver for Onedata

**Warning: This is only a proof of concept. It should not be used in production environments!**

This repository contains the CSI driver for [Onedata](https://onedata.org/). It allows to mount directories using a oneclient connection.

## Usage

### Deploy CSI-driver (for cluster administrator)

Deploy the whole directory `deploy/kubernetes`.
This installs the csi controller and node plugin and a appropriate storage class for the csi driver.

```bash
kubectl apply -f deploy/kubernetes
```

### Create pod (for users)

To use the csi driver create a secret, persistent volume and persistent volume claim like the example one:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: secret-onedata-example
#  labels:
#    cerit-onedata-secret: "yes" # this driver is designed for the CERIT-SC cloud infrastructure
data:
  host: <HOST_ONEPROVIDER-URL> # host in base64 form
  onedata_token: <ONEDATA_TOKEN> # token in base64 form
  space_id: <SPACE-ID> # space-id in base64 form
  oneclient_options: LS1mb3JjZS1kaXJlY3QtaW8K # "--force-direct-io" - Recommended for better performance, base64
type: Opaque

---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: pv-onedata-example
spec:
  accessModes:
    - ReadWriteMany
  capacity:
    storage: 100Gi
  storageClassName: "onedata"
  csi:
    driver: csi-onedata
    nodePublishSecretRef:
      name: secret-onedata-example
      namespace: <YOUR-NAMESPACE>
    volumeHandle: pv-onedata-example

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc-onedata-example
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 100Gi
  storageClassName: onedata
  volumeName: pv-onedata-example
```

Then mount the volume into a pod:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - image: nginx
    imagePullPolicy: Always
    name: nginx
    ports:
    - containerPort: 80
      protocol: TCP
    volumeMounts:
      - mountPath: /var/www
        name: data-onedata
  volumes:
  - name: data-onedata
    persistentVolumeClaim:
      claimName: pvc-onedata-example
```
