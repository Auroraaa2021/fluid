/*
Copyright 2022 The Fluid Authors

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

package goosefs

import (
	"github.com/fluid-cloudnative/fluid/pkg/common"
	volumeHelper "github.com/fluid-cloudnative/fluid/pkg/utils/dataset/volume"
)

// CreateVolume creates volume
func (e *GooseFSEngine) CreateVolume() (err error) {
	if e.runtime == nil {
		e.runtime, err = e.getRuntime()
		if err != nil {
			return
		}
	}

	err = e.createFusePersistentVolume()
	if err != nil {
		return err
	}

	err = e.createFusePersistentVolumeClaim()
	if err != nil {
		return err
	}

	err = e.createHCFSPersistentVolume()
	if err != nil {
		return err
	}

	return nil

}

// createFusePersistentVolume
func (e *GooseFSEngine) createFusePersistentVolume() (err error) {
	runtimeInfo, err := e.getRuntimeInfo()
	if err != nil {
		return err
	}

	return volumeHelper.CreatePersistentVolumeForRuntime(e.Client,
		runtimeInfo,
		e.getMountPoint(),
		common.GooseFSMountType,
		e.Log)

}

// createFusePersistentVolume
func (e *GooseFSEngine) createFusePersistentVolumeClaim() (err error) {
	runtimeInfo, err := e.getRuntimeInfo()
	if err != nil {
		return err
	}

	return volumeHelper.CreatePersistentVolumeClaimForRuntime(e.Client, runtimeInfo, e.Log)

}

// createHCFSVolume (TODO: chrisydxie)
func (e *GooseFSEngine) createHCFSPersistentVolume() (err error) {
	return nil
}

// createHCFSVolume (TODO: chrisydxie)
// func (e *GooseFSEngine) createHCFSPersistentVolumeClaim() (err error) {
// 	return nil
// }
