// Copyright 2023 Sun Quan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package register

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// consul node
type Node struct {
	Name           string `json:"Node"`                     // hostname
	AdmIp          string `json:"Address"`                  // adm ip
	SkipNodeUpdate bool   `json:"SkipNodeUpdate,omitempty"` // skip node update
}

// provider
func NewNode(admIp string) (*Node, error) {
	hostName, _ := os.Hostname()
	return &Node{
		Name:  hostName,
		AdmIp: admIp,
	}, nil
}

func (n *Node) String() string {
	return fmt.Sprintf("%s %s", n.Name, n.AdmIp)
}

// only register node
func (n *Node) Register(consulAddr string) error {
	if !strings.HasPrefix(consulAddr, "http://") {
		consulAddr = "http://" + consulAddr
	}
	registerPath := "/v1/catalog/register"
	u, err := url.Parse(consulAddr)
	if err != nil {
		return fmt.Errorf("node url.Parse err: %w", err)
	}
	u = u.JoinPath(registerPath)

	n.SkipNodeUpdate = true
	bys, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("node json.Marshal err: %w", err)
	}
	reader := bytes.NewReader(bys)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	req, err := http.NewRequest("PUT", u.String(), reader)
	if err != nil {
		return fmt.Errorf("node http.NewRequest err: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	defer req.Body.Close()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("node client.Do err: %w", err)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("node register failed with status code: %d", resp.StatusCode)
	}
	return nil
}

// 管理地址所在interface规则，en0
// 选取最小的ip
func hostAdmIp(intfList []string) (string, error) {
	var (
		intf *net.Interface
		err  error
	)
	for i := 0; i < len(intfList); i++ {
		intf, err = net.InterfaceByName(intfList[i]) // bond0, eth0
		if err != nil {
			continue
		}
		break
	}
	// no intf found
	if intf == nil && err != nil {
		return "", err
	}

	addrs, _ := intf.Addrs()
	addrList := make([]netip.Addr, 0)
	// transfer net.Addr to netip.Addr
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			addr, ok := netip.AddrFromSlice(ipNet.IP.To4())
			if !ok {
				fmt.Println("AddrFromSlice failed.")
				continue
			}
			addrList = append(addrList, addr)
		}
	}
	// no ip found
	if len(addrList) == 0 {
		return "", errors.New("no ip found")
	}
	addrList = sortAddrList(addrList)
	return addrList[0].String(), nil
}

func sortAddrList(addrList []netip.Addr) []netip.Addr {
	sort.Slice(addrList, func(i, j int) bool {
		return addrList[i].Less(addrList[j])
	})
	return addrList
}
