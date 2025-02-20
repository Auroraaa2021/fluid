/*
Copyright 2021 The Fluid Authors.

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

package juicefs

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	datav1alpha1 "github.com/fluid-cloudnative/fluid/api/v1alpha1"
	"github.com/fluid-cloudnative/fluid/pkg/common"
	"github.com/fluid-cloudnative/fluid/pkg/utils"
	"github.com/fluid-cloudnative/fluid/pkg/utils/tieredstore"
)

func (j *JuiceFSEngine) transformResourcesForFuse(runtime *datav1alpha1.JuiceFSRuntime, value *JuiceFS) {

	if runtime.Spec.Fuse.Resources.Limits == nil {
		j.Log.Info("skip setting memory limit")
		return
	}

	if _, found := runtime.Spec.Fuse.Resources.Limits[corev1.ResourceMemory]; !found {
		j.Log.Info("skip setting memory limit")
		return
	}

	value.Fuse.Resources = utils.TransformRequirementsToResources(runtime.Spec.Fuse.Resources)

	runtimeInfo, err := j.getRuntimeInfo()
	if err != nil {
		j.Log.Error(err, "failed to transformResourcesForFuse")
	}
	storageMap := tieredstore.GetLevelStorageMap(runtimeInfo)

	j.Log.Info("transformFuse", "storageMap", storageMap)

	// TODO(iluoeli): it should be xmx + direct memory
	memLimit := resource.MustParse("50Gi")
	if quantity, exists := runtime.Spec.Fuse.Resources.Limits[corev1.ResourceMemory]; exists && !quantity.IsZero() {
		memLimit = quantity
	}

	for key, requirement := range storageMap {
		if value.Fuse.Resources.Limits == nil {
			value.Fuse.Resources.Limits = make(common.ResourceList)
		}
		if key == common.MemoryCacheStore {
			req := requirement.DeepCopy()

			memLimit.Add(req)

			j.Log.Info("update the requiremnet for memory", "requirement", memLimit)

		}
		// } else if key == common.DiskCacheStore {
		// 	req := requirement.DeepCopy()
		// 	e.Log.Info("update the requiremnet for disk", "requirement", req)
		// 	value.Fuse.Resources.Limits[corev1.ResourceEphemeralStorage] = req.String()
		// }
	}
	if value.Fuse.Resources.Limits != nil {
		value.Fuse.Resources.Limits[corev1.ResourceMemory] = memLimit.String()
	}

}

func (j *JuiceFSEngine) transformResourcesForWorker(runtime *datav1alpha1.JuiceFSRuntime, value *JuiceFS) {
	if runtime.Spec.Worker.Resources.Limits == nil {
		j.Log.Info("skip setting memory limit")
		return
	}

	if _, found := runtime.Spec.Worker.Resources.Limits[corev1.ResourceMemory]; !found {
		j.Log.Info("skip setting memory limit")
		return
	}

	value.Worker.Resources = utils.TransformRequirementsToResources(runtime.Spec.Worker.Resources)

	if quantity, exists := runtime.Spec.Worker.Resources.Limits[corev1.ResourceMemory]; exists && !quantity.IsZero() {
		value.Worker.Resources.Limits[corev1.ResourceMemory] = quantity.String()
	}
}
