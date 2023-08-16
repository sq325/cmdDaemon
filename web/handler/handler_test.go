package handler

import (
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestLis(t *testing.T) {
	Listen()
}

func register() {
	http.HandleFunc("/reload", Reload)
}
func Reload(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("Reload"))
}

func Listen() {
	register()
	http.ListenAndServe(":8080", nil)
}

func TestPortCmdMap(t *testing.T) {
	var spacePattern = regexp.MustCompile(`\s+`)

	// run lsof
	allTCP, err := exec.Command("sh", "-c", "lsof -PiTCP | grep LISTEN").Output()
	if err != nil {
		t.Error(err)
	}
	// t.Log(string(allTCP))
	allTCPSlice := strings.Split(string(allTCP), "\n")
	for i, line := range allTCPSlice {
		lineSlice := spacePattern.Split(line, -1)
		if len(lineSlice) < 2 {
			continue
		}
		t.Log(i, len(lineSlice), lineSlice[len(lineSlice)-2])
	}
}

func Test_pidAddr(t *testing.T) {
	t.Log(pidAddr())
}
