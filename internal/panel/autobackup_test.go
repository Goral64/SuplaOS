package panel

import (
	"testing"

	"github.com/Goral64/SuplaOS/internal/autobackup"
)

func TestAutoBackupMerge(t *testing.T) {
	old := autobackup.Default()
	old.Password = "stare-haslo"
	old.SMB = autobackup.SMB{Share: "//nas/a", Username: "jan", Password: "smb-haslo"}

	req := autoBackupRequest{Settings: *old}
	req.Settings.Password, req.Settings.SMB.Password = "", ""
	if s := req.merge(old); s.Password != "stare-haslo" || s.SMB.Password != "smb-haslo" {
		t.Errorf("empty passwords not kept: %+v", s)
	}
	req.Password, req.SMBPassword = "nowe-haslo", "nowe-smb"
	if s := req.merge(old); s.Password != "nowe-haslo" || s.SMB.Password != "nowe-smb" {
		t.Errorf("new passwords: %+v", s)
	}
	req = autoBackupRequest{Settings: *old}
	req.Settings.SMB.Share = "//inny/b"
	req.Settings.Plain = true
	if s := req.merge(old); s.Password != "" || s.SMB.Password != "" {
		t.Errorf("plain or another share kept a password: %+v", s)
	}
}
