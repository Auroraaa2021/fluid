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

package utils

import (
	datav1alpha1 "github.com/fluid-cloudnative/fluid/api/v1alpha1"
	"github.com/fluid-cloudnative/fluid/pkg/common"
)

func GetRuntimeByCategory(runtimes []datav1alpha1.Runtime, category common.Category) (index int, runtime *datav1alpha1.Runtime) {
	if runtimes == nil {
		return -1, nil
	}
	for i := range runtimes {
		if runtimes[i].Category == category {
			return i, &runtimes[i]
		}
	}
	return -1, nil
}
