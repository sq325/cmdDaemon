// Copyright 2023 Sun Quan
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package register

import (
	"net"
	"testing"
)

func Test_consulConf(t *testing.T) {
	consulConf()
}

func TestMain(t *testing.T) {
	// t.Log(os.Hostname())
	// t.Log(net.InterfaceAddrs())
	inters, _ := net.Interfaces()
	for _, inter := range inters {
		t.Log(inter)
	}
}

func TestSplitHostPort(t *testing.T) {
	host, port, _ := net.SplitHostPort("*:8080")
	t.Log(port)
	t.Log(host)
}
