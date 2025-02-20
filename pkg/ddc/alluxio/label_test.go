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

package alluxio

import "testing"

func TestGetCommonLabelname(t *testing.T) {
	testCases := []struct {
		name      string
		namespace string
		out       string
	}{
		{
			name:      "hbase",
			namespace: "fluid",
			out:       "fluid.io/s-fluid-hbase",
		},
		{
			name:      "hadoop",
			namespace: "fluid",
			out:       "fluid.io/s-fluid-hadoop",
		},
		{
			name:      "common",
			namespace: "default",
			out:       "fluid.io/s-default-common",
		},
	}
	for _, testCase := range testCases {
		engine := &AlluxioEngine{
			name:      testCase.name,
			namespace: testCase.namespace,
		}
		out := engine.getCommonLabelname()
		if out != testCase.out {
			t.Errorf("in: %s-%s, expect: %s, got: %s", testCase.namespace, testCase.name, testCase.out, out)
		}
	}
}
