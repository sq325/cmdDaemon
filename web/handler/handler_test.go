package handler

import (
	"net/http"
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
