package system

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
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
	if !SSHPasswordLogin() && !slices.ContainsFunc(keep, func(l string) bool { _, ok := parseKeyLine(l); return ok }) {
		return errors.New("SSH accepts only keys: turn password logins on first")
	}
	return writeAuthorizedKeys(keep)
}

// sshKeysOnlyFile turns password logins over SSH off. sshd reads it before
// 10-suplaos.conf and takes the first value it finds.
var sshKeysOnlyFile = "/etc/ssh/sshd_config.d/05-suplaos-keys-only.conf"

// reloadSSH checks the configuration of sshd and makes it read it again.
var reloadSSH = func() error {
	if err := run("sshd", "-t"); err != nil {
		return err
	}
	return run("systemctl", "reload", "ssh")
}

// SSHPasswordLogin tells whether SSH takes the password, not only keys.
func SSHPasswordLogin() bool {
	_, err := os.Stat(sshKeysOnlyFile)
	return err != nil
}

// SetSSHPasswordLogin turns password logins over SSH on or off. Off needs a
// key, so that the account can still log in. Logins already open stay.
func SetSSHPasswordLogin(on bool) error {
	if on {
		if err := os.Remove(sshKeysOnlyFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return reloadSSH()
	}
	keys, err := SSHKeys()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return errors.New("add an SSH key first")
	}
	data := "# Written by the SuplaOS panel: only SSH keys log in.\nPasswordAuthentication no\nKbdInteractiveAuthentication no\n"
	if err := os.WriteFile(sshKeysOnlyFile, []byte(data), 0o644); err != nil {
		return err
	}
	if err := reloadSSH(); err != nil {
		os.Remove(sshKeysOnlyFile)
		return err
	}
	return nil
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
