/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package plugins

import (
	"fmt"
	"github.com/fluid-cloudnative/fluid/api/v1alpha1"
	"github.com/fluid-cloudnative/fluid/pkg/common"
	"github.com/fluid-cloudnative/fluid/pkg/ddc/base"
	"github.com/fluid-cloudnative/fluid/pkg/utils"
	"github.com/fluid-cloudnative/fluid/pkg/utils/dataset/volume"
	"github.com/fluid-cloudnative/fluid/pkg/utils/kubeclient"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"os"
	"os/exec"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
	"syscall"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
)

const (
	AllowPatchStaleNodeEnv = "ALLOW_PATCH_STALE_NODE"
)

type nodeServer struct {
	nodeId string
	*csicommon.DefaultNodeServer
	client    client.Client
	apiReader client.Reader
	mutex     sync.Mutex
	node      *v1.Node
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	glog.Infof("NodePublishVolumeRequest is %v", req)
	targetPath := req.GetTargetPath()

	isMount, err := utils.IsMounted(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			} else {
				glog.Infof("MkdirAll successful. %v", targetPath)
			}
			//isMount = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if isMount {
		glog.Infof("It's already mounted to %v", targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	} else {
		glog.Infof("Try to mount to %v", targetPath)
	}

	// 0. check if read only
	readOnly := false
	if req.GetVolumeCapability() == nil {
		glog.Infoln("Volume Capability is nil")
	} else {
		mode := req.GetVolumeCapability().GetAccessMode().GetMode()
		if mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			readOnly = true
			glog.Infof("Set the mount option readonly=%v", readOnly)
		}
	}

	// mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	// if req.GetReadonly() {
	// 	mountOptions = append(mountOptions, "ro")
	// }

	/*
	   https://docs.alluxio.io/os/user/edge/en/api/POSIX-API.html
	   https://github.com/Alluxio/alluxio/blob/master/integration/fuse/bin/alluxio-fuse
	*/

	fluidPath := req.GetVolumeContext()[common.VolumeAttrFluidPath]
	mountType := req.GetVolumeContext()[common.VolumeAttrMountType]
	if fluidPath == "" {
		// fluidPath = fmt.Sprintf("/mnt/%s", req.)
		return nil, status.Error(codes.InvalidArgument, "fluid_path is not set")
	}
	if mountType == "" {
		// default mountType is ALLUXIO_MOUNT_TYPE
		mountType = common.ALLUXIO_MOUNT_TYPE
	}

	// 1. Wait the runtime fuse ready
	err = utils.CheckMountReady(fluidPath, mountType)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	args := []string{"--bind"}
	// if len(mountOptions) > 0 {
	// 	args = append(args, "-o", strings.Join(mountOptions, ","))
	// }

	if readOnly {
		args = append(args, "-o", "ro", fluidPath, targetPath)
	} else {
		args = append(args, fluidPath, targetPath)
	}
	command := exec.Command("mount", args...)

	glog.V(4).Infoln(command)
	stdoutStderr, err := command.CombinedOutput()
	glog.V(4).Infoln(string(stdoutStderr))
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		glog.V(4).Infof("Succeed in binding %s to %s", fluidPath, targetPath)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()

	// targetPath may be mount bind many times when mount point recovered.
	// umount until it's not mounted.
	mounter := mount.New("")
	for {
		notMount, err := mounter.IsLikelyNotMountPoint(targetPath)
		if err != nil {
			glog.V(3).Infoln(err)
			if corrupted := mount.IsCorruptedMnt(err); !corrupted {
				return nil, errors.Wrapf(err, "NodeUnpublishVolume: stat targetPath %s error %v", targetPath, err)
			}
		}
		if notMount {
			glog.V(3).Infof("umount:%s success", targetPath)
			break
		}

		glog.V(3).Infof("umount:%s", targetPath)
		err = mounter.Unmount(targetPath)
		if err != nil {
			glog.V(3).Infoln(err)
			return nil, errors.Wrapf(err, "NodeUnpublishVolume: umount targetPath %s error %v", targetPath, err)
		}
	}

	err := mount.CleanupMountPoint(req.GetTargetPath(), mount.New(""), false)
	if err != nil {
		glog.V(3).Infoln(err)
	} else {
		glog.V(4).Infof("Succeed in umounting  %s", targetPath)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// The lock is to ensure CSI plugin labels the node in correct order
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	// 1. get runtime namespace and name
	// A nil volumeContext is passed because unlike csi.NodeStageVolumeRequest, csi.NodeUnstageVolumeRequest has
	// no volume context attribute.
	namespace, name, err := ns.getRuntimeNamespacedName(nil, req.GetVolumeId())
	if err != nil {
		glog.Errorf("NodeUnstageVolume: can't get runtime namespace and name given (volumeContext: nil, volumeId: %s): %v", req.GetVolumeId(), err)
		return nil, errors.Wrapf(err, "NodeUnstageVolume: can't get namespace and name by volume id %s", req.GetVolumeId())
	}

	// 2. Check fuse clean policy. If clean policy is set to OnRuntimeDeleted, there is no
	// need to clean fuse eagerly.
	runtimeInfo, err := base.GetRuntimeInfo(ns.client, name, namespace)
	if err != nil {
		return nil, errors.Wrap(err, "NodeUnstageVolume: can't get fuse clean policy")
	}

	var shouldCleanFuse bool
	cleanPolicy := runtimeInfo.GetFuseCleanPolicy()
	glog.Infof("Using %s clean policy for runtime %s in namespace %s", cleanPolicy, runtimeInfo.GetName(), runtimeInfo.GetNamespace())
	switch cleanPolicy {
	case v1alpha1.OnDemandCleanPolicy:
		shouldCleanFuse = true
	case v1alpha1.OnRuntimeDeletedCleanPolicy:
		shouldCleanFuse = false
	default:
		return nil, errors.Errorf("Unknown Fuse clean policy: %s", cleanPolicy)
	}

	if !shouldCleanFuse {
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	// 3. check if the path is mounted
	inUse, err := checkMountInUse(req.GetVolumeId())
	if err != nil {
		return nil, errors.Wrap(err, "NodeUnstageVolume: can't check mount in use")
	}
	if inUse {
		return nil, fmt.Errorf("NodeUnstageVolume: can't stop fuse cause it's in use")
	}

	// 4. remove label on node
	// Once the label is removed, fuse pod on corresponding node will be terminated
	// since node selector in the fuse daemonSet no longer matches.
	// TODO: move all the label keys into a util func
	fuseLabelKey := common.LabelAnnotationFusePrefix + namespace + "-" + name
	var labelsToModify common.LabelsToModify
	labelsToModify.Delete(fuseLabelKey)

	node, err := ns.getNode()
	if err != nil {
		glog.Errorf("NodeUnstageVolume: can't get node %s: %v", ns.nodeId, err)
		return nil, errors.Wrapf(err, "NodeUnstageVolume: can't get node %s", ns.nodeId)
	}

	_, err = utils.ChangeNodeLabelWithPatchMode(ns.client, node, labelsToModify)
	if err != nil {
		glog.Errorf("NodeUnstageVolume: error when patching labels on node %s: %v", ns.nodeId, err)
		return nil, errors.Wrapf(err, "NodeUnstageVolume: error when patching labels on node %s", ns.nodeId)
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	// The lock is to ensure CSI plugin labels the node in correct order
	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	// 1. get runtime namespace and name
	namespace, name, err := ns.getRuntimeNamespacedName(req.GetVolumeContext(), req.GetVolumeId())
	if err != nil {
		glog.Errorf("NodeStageVolume: can't get runtime namespace and name given (volumeContext: %v, volumeId: %s): %v", req.GetVolumeContext(), req.GetVolumeId(), err)
		return nil, errors.Wrapf(err, "NodeStageVolume: can't get namespace and name by volume id %s", req.GetVolumeId())
	}

	// 2. Label node
	fuseLabelKey := common.LabelAnnotationFusePrefix + namespace + "-" + name
	var labelsToModify common.LabelsToModify
	labelsToModify.Add(fuseLabelKey, "true")

	node, err := ns.getNode()
	if err != nil {
		glog.Errorf("NodeStageVolume: can't get node %s: %v", ns.nodeId, err)
		return nil, errors.Wrapf(err, "NodeStageVolume: can't get node %s", ns.nodeId)
	}

	_, err = utils.ChangeNodeLabelWithPatchMode(ns.client, node, labelsToModify)
	if err != nil {
		glog.Errorf("NodeStageVolume: error when patching labels on node %s: %v", ns.nodeId, err)
		return nil, errors.Wrapf(err, "NodeStageVolume: error when patching labels on node %s", ns.nodeId)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	glog.V(5).Infof("Using default NodeGetCapabilities")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

// getRuntimeNamespacedName first checks volume context for runtime's namespace and name as a fast path.
// If not found, it takes a fallback to query API Server and to parse the PV information.
func (ns *nodeServer) getRuntimeNamespacedName(volumeContext map[string]string, volumeId string) (namespace string, name string, err error) {
	// Fast path to check namespace && name in volume context
	if len(volumeContext) != 0 {
		runtimeName, nameFound := volumeContext[common.VolumeAttrName]
		runtimeNamespace, nsFound := volumeContext[common.VolumeAttrNamespace]
		if nameFound && nsFound {
			glog.V(3).Infof("Get runtime namespace(%s) and name(%s) from volume context", runtimeNamespace, runtimeName)
			return runtimeNamespace, runtimeName, nil
		}
	}

	// Fallback: query API Server to get namespaced name
	glog.Infof("Get runtime namespace and name directly from api server with volumeId %s", volumeId)
	return volume.GetNamespacedNameByVolumeId(ns.apiReader, volumeId)
}

// getNode first checks cached node
func (ns *nodeServer) getNode() (node *v1.Node, err error) {
	// Default to allow patch stale node info
	if envVar, found := os.LookupEnv(AllowPatchStaleNodeEnv); !found || "true" == envVar {
		if ns.node != nil {
			glog.V(3).Infof("Found cached node %s", ns.node.Name)
			return ns.node, nil
		}
	}

	if node, err = kubeclient.GetNode(ns.apiReader, ns.nodeId); err != nil {
		return nil, err
	}
	glog.V(1).Infof("Got node %s from api server", node.Name)
	ns.node = node
	return ns.node, nil
}

func checkMountInUse(volumeName string) (bool, error) {
	var inUse bool
	glog.Infof("Try to check if the volume %s is being used", volumeName)
	if volumeName == "" {
		return inUse, errors.New("volumeName is not specified")
	}

	// TODO: refer to https://github.com/kubernetes-sigs/alibaba-cloud-csi-driver/blob/4fcb743220371de82d556ab0b67b08440b04a218/pkg/oss/utils.go#L72
	// for a better implementation
	command := exec.Command("/usr/local/bin/check_bind_mounts.sh", volumeName)
	glog.Infoln(command)

	stdoutStderr, err := command.CombinedOutput()
	glog.Infoln(string(stdoutStderr))

	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exitStatus := status.ExitStatus()
				if exitStatus == 1 {
					// grep not found any mount entry
					err = nil
					inUse = false
				}
			}
		}
	} else {
		waitStatus := command.ProcessState.Sys().(syscall.WaitStatus)
		if waitStatus.ExitStatus() == 0 {
			inUse = true
		}
	}

	return inUse, err
}
