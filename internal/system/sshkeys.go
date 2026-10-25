package system

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// authorizedKeys is the file with the SSH keys of the supla account.
var authorizedKeys = "/home/" + User + "/.ssh/authorized_keys"

// SSHKey is a public key that may log in over SSH as the supla user.
type SSHKey struct {
	Type        string `json:"type"`
	Comment     string `json:"comment"`
	Fingerprint string `json:"fingerprint"`
}

func parseKeyLine(line string) (*SSHKey, bool) {
	pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return nil, false
	}
	return &SSHKey{Type: pub.Type(), Comment: comment, Fingerprint: ssh.FingerprintSHA256(pub)}, true
}

func readAuthorizedKeys() ([]string, error) {
	data, err := os.ReadFile(authorizedKeys)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n"), nil
}

// SSHKeys lists the keys in authorized_keys.
func SSHKeys() ([]SSHKey, error) {
	lines, err := readAuthorizedKeys()
	if err != nil {
		return nil, err
	}
	keys := []SSHKey{}
	for _, line := range lines {
		if k, ok := parseKeyLine(line); ok {
			keys = append(keys, *k)
		}
	}
	return keys, nil
}

// AddSSHKey adds a public key in the authorized_keys format (one line, as
// in id_ed25519.pub).
func AddSSHKey(text string) (*SSHKey, error) {
	text = strings.TrimSpace(text)
	if strings.ContainsAny(text, "\r\n") {
		return nil, errors.New("give one key, in one line")
	}
	k, ok := parseKeyLine(text)
	if !ok {
		return nil, errors.New("not a public SSH key, expected e.g. ssh-ed25519 AAAA... name")
	}
	if strings.Contains(text, "PRIVATE KEY") {
		return nil, errors.New("this is a private key, give the public one (.pub)")
	}
	lines, err := readAuthorizedKeys()
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		if old, ok := parseKeyLine(line); ok && old.Fingerprint == k.Fingerprint {
			return nil, errors.New("this key is already there")
		}
	}
	if err := writeAuthorizedKeys(append(lines, text)); err != nil {
		return nil, err
	}
	return k, nil
}

// DeleteSSHKey removes the key with the fingerprint; other lines stay.
func DeleteSSHKey(fingerprint string) error {
	lines, err := readAuthorizedKeys()
	if err != nil {
		return err
	}
	var keep []string
	found := false
	for _, line := range lines {
		if k, ok := parseKeyLine(line); ok && k.Fingerprint == fingerprint {
			found = true
			continue
		}
		keep = append(keep, line)
	}
	if !found {
		return fmt.Errorf("no key %s", fingerprint)
	}
	return writeAuthorizedKeys(keep)
}

// writeAuthorizedKeys writes the file as sshd wants it: owned by the user,
// not writable by others, also the .ssh directory.
func writeAuthorizedKeys(lines []string) error {
	dir := filepath.Dir(authorizedKeys)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var data string
	for _, l := range lines {
		if l != "" {
			data += l + "\n"
		}
	}
	tmp := authorizedKeys + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o600); err != nil {
		return err
	}
	if u, err := user.Lookup(User); err == nil && os.Geteuid() == 0 {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		os.Chown(dir, uid, gid)
		os.Chown(tmp, uid, gid)
	}
	return os.Rename(tmp, authorizedKeys)
}
