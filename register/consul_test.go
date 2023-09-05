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
	"errors"
	"net"
	"testing"
)

func Test_consulConf(t *testing.T) {
	consulConf()
}

func TestSplitHostPort(t *testing.T) {
	host, port, _ := net.SplitHostPort("*:8080")
	t.Log(port)
	t.Log(host)
}

func Test_hostAdmIp(t *testing.T) {
	intf, err := net.InterfaceByName("ent0") // bond0, eth0
	if errors.Is(err, net.InvalidAddrError()) {
		t.Log(err)
		return
	}
	t.Log(intf.Name)

}
