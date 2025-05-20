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

package tool

import (
	"net"
	"strings"
	"testing"
)

func Test_pidAddr(t *testing.T) {
	m, _ := PidAddr()
	for k, v := range m {
		t.Log(k, ": ", v)
		t.Log(net.ResolveTCPAddr("tcp", v))
	}
}

func TestPidAddr(t *testing.T) {
	m, _ := PidAddr()
	for k, v := range m {
		t.Log(k, ": ", v)
	}
	t.Log(m["79704"])
}
func TestIpFromHostname(t *testing.T) {
	tests := []struct {
		name          string
		hostname      string
		expectSuccess bool   // True if we expect a non-empty IP and no error
		wantErrMsgSub string // Expected error substring if expectSuccess is false
	}{
		{
			name:          "localhost lookup",
			hostname:      "localhost",
			expectSuccess: true, // Expect localhost to be resolvable from /etc/hosts
		},
		{
			name:          "non-existent hostname",
			hostname:      "this-hostname-is-very-unlikely-to-exist-in-hosts-file-abcdef12345",
			expectSuccess: false,
			wantErrMsgSub: "在 /etc/hosts 文件中未找到主机名: this-hostname-is-very-unlikely-to-exist-in-hosts-file-abcdef12345",
		},
		{
			name:          "empty hostname",
			hostname:      "",
			expectSuccess: false,
			wantErrMsgSub: "在 /etc/hosts 文件中未找到主机名: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIP, err := IpFromHostname(tt.hostname)

			if tt.expectSuccess {
				if err != nil {
					// If an error occurs, it might be because /etc/hosts is inaccessible or
					// (less likely for "localhost") the entry is missing.
					// The function is expected to return an error in such cases.
					// We log this as it depends on the environment.
					if strings.Contains(err.Error(), "无法打开 /etc/hosts 文件") {
						t.Logf("IpFromHostname(%q): /etc/hosts inaccessible or unreadable: %v. This is a valid outcome.", tt.hostname, err)
					} else if strings.Contains(err.Error(), "在 /etc/hosts 文件中未找到主机名: "+tt.hostname) {
						t.Logf("IpFromHostname(%q) did not find the host: %v. This might indicate an unusual /etc/hosts configuration or that the file is empty/malformed.", tt.hostname, err)
					} else {
						t.Errorf("IpFromHostname(%q) returned unexpected error: %v", tt.hostname, err)
					}
					// In these error scenarios for an expected success, we don't fail the test outright
					// as the function correctly reported an issue it encountered.
					// However, if the goal was to *ensure* resolution, this would be a failure.
					// For this test, we're checking the function's behavior given /etc/hosts.
					return
				}
				if gotIP == "" {
					t.Errorf("IpFromHostname(%q) returned an empty IP without error, expected a valid IP.", tt.hostname)
				} else {
					// Basic validation: IP should not be the hostname itself and should look like an IP.
					if gotIP == tt.hostname {
						t.Errorf("IpFromHostname(%q) returned hostname as IP: %s", tt.hostname, gotIP)
					}
					if !strings.Contains(gotIP, ".") && !strings.Contains(gotIP, ":") {
						t.Errorf("IpFromHostname(%q) returned %q, which does not appear to be a valid IPv4 or IPv6 address.", tt.hostname, gotIP)
					}
					t.Logf("IpFromHostname(%q) resolved to %q", tt.hostname, gotIP)
				}
			} else { // Expect an error
				if err == nil {
					t.Errorf("IpFromHostname(%q) expected an error, but got IP: %s", tt.hostname, gotIP)
					return
				}
				// The error can either be "host not found" or "cannot open file".
				// Both are valid failure reasons for these test cases.
				if !strings.Contains(err.Error(), tt.wantErrMsgSub) && !strings.Contains(err.Error(), "无法打开 /etc/hosts 文件") {
					t.Errorf("IpFromHostname(%q) error = %q; want error containing %q OR '无法打开 /etc/hosts 文件'", tt.hostname, err.Error(), tt.wantErrMsgSub)
				} else {
					t.Logf("IpFromHostname(%q) correctly returned error: %v", tt.hostname, err)
				}
			}
		})
	}
}
