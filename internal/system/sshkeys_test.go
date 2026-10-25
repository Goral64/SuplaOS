package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	edKey  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl jan@laptop"
	edKey2 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB7BkzN0Hzr6p5f4kO2c3vG8Iu1fN0QpX2wzk9KfWb2l"
)

func TestSSHKeys(t *testing.T) {
	old := authorizedKeys
	authorizedKeys = filepath.Join(t.TempDir(), ".ssh", "authorized_keys")
	t.Cleanup(func() { authorizedKeys = old })

	if keys, err := SSHKeys(); err != nil || len(keys) != 0 {
		t.Fatalf("no file: %v %v", keys, err)
	}
	k, err := AddSSHKey("  " + edKey + "\n")
	if err != nil || k.Type != "ssh-ed25519" || k.Comment != "jan@laptop" || !strings.HasPrefix(k.Fingerprint, "SHA256:") {
		t.Fatalf("add: %+v %v", k, err)
	}
	if _, err := AddSSHKey(edKey); err == nil {
		t.Error("duplicate accepted")
	}
	for _, bad := range []string{"", "ssh-ed25519 notbase64", edKey + "\n" + edKey2, "-----BEGIN OPENSSH PRIVATE KEY-----"} {
		if _, err := AddSSHKey(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// A line written by hand stays.
	f, _ := os.OpenFile(authorizedKeys, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("# laptop\n")
	f.Close()
	_, err = AddSSHKey(edKey2)
	must(t, err)
	if keys, _ := SSHKeys(); len(keys) != 2 {
		t.Fatalf("keys %v", keys)
	}
	must(t, DeleteSSHKey(k.Fingerprint))
	data, _ := os.ReadFile(authorizedKeys)
	if string(data) != "# laptop\n"+edKey2+"\n" {
		t.Errorf("file:\n%s", data)
	}
	if DeleteSSHKey(k.Fingerprint) == nil {
		t.Error("deleted twice")
	}
	if st, _ := os.Stat(authorizedKeys); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
