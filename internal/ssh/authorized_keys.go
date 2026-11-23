package ssh

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/rs/zerolog/log"
)

// authorizedKeysOptions are the SSH restrictions applied to every QingyuGist key
// entry: the connection may only run the forced git command, nothing else.
const authorizedKeysOptions = "no-port-forwarding,no-x11-forwarding,no-agent-forwarding,no-pty"

// Markers delimiting the block QingyuGist manages inside the authorized_keys file.
// Anything outside them (e.g. hand-added keys) is preserved across syncs.
const (
	authorizedKeysBegin = "# --- qingyugist managed keys start (do not edit) ---"
	authorizedKeysEnd   = "# --- qingyugist managed keys end ---"
	legacyKeysBegin     = "# --- " + config.LegacyExecutableName + " managed keys start (do not edit) ---"
	legacyKeysEnd       = "# --- " + config.LegacyExecutableName + " managed keys end ---"
)

var authorizedKeysMu sync.Mutex

// AuthorizedKeysLine builds a single authorized_keys entry for a QingyuGist key:
// a forced command running `qingyugist shell <keyID>` (so the daemon knows which
// key is connecting), restricted to git, followed by the public key itself.
//
// It is shared by the `keys` command (AuthorizedKeysCommand mode) and the
// managed authorized_keys file writer. execPath is the absolute path to the
// QingyuGist binary.
func AuthorizedKeysLine(execPath string, keyID uint, pubKey string) string {
	var cmd strings.Builder
	cmd.WriteString(execPath)
	cmd.WriteString(" shell ")
	cmd.WriteString(strconv.FormatUint(uint64(keyID), 10))

	return `command="` + cmd.String() + `",` + authorizedKeysOptions + " " + strings.TrimSpace(pubKey)
}

// SyncAuthorizedKeys rewrites QingyuGist's managed block in the configured
// authorized_keys file from every SSH key in the database, preserving any other
// lines in the file. It is a no-op unless host mode with a configured file path
// (config.SshManagesAuthorizedKeys). Safe for concurrent callers.
func SyncAuthorizedKeys() error {
	if !config.C.SshManagesAuthorizedKeys() {
		return nil
	}

	authorizedKeysMu.Lock()
	defer authorizedKeysMu.Unlock()

	keys, err := db.GetAllSSHKeys()
	if err != nil {
		return err
	}

	// Reuse the stable symlinks (as the Git hooks do) so the forced command keeps
	// working across binary upgrades.
	exe := filepath.Join(config.GetHomeDir(), "symlinks", "qingyugist")

	var block strings.Builder
	block.WriteString(authorizedKeysBegin + "\n")
	for _, k := range keys {
		block.WriteString(AuthorizedKeysLine(exe, k.ID, k.Content) + "\n")
	}
	block.WriteString(authorizedKeysEnd + "\n")

	return writeAuthorizedKeysFile(config.C.SshAuthorizedKeysFile, block.String())
}

// SyncAuthorizedKeysLogged runs SyncAuthorizedKeys and logs any error. It is for
// callers that should not fail their own operation when the sync fails: the
// database is already updated, and the next change or a restart reconciles.
func SyncAuthorizedKeysLogged() {
	if err := SyncAuthorizedKeys(); err != nil {
		log.Error().Err(err).Msg("Failed to sync the authorized_keys file")
	}
}

// writeAuthorizedKeysFile replaces the QingyuGist-managed block (between the
// marker lines) in path with managedBlock, preserving all other lines, and
// writes the result atomically with sshd-compatible permissions (dir 0700,
// file 0600 — sshd's StrictModes ignores looser ones).
func writeAuthorizedKeysFile(path, managedBlock string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	var preserved []string
	if existing, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(existing)
		inBlock := false
		for scanner.Scan() {
			line := scanner.Text()
			switch line {
			case authorizedKeysBegin, legacyKeysBegin:
				inBlock = true
			case authorizedKeysEnd, legacyKeysEnd:
				inBlock = false
			default:
				if !inBlock {
					preserved = append(preserved, line)
				}
			}
		}
		_ = existing.Close()
		if err := scanner.Err(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	var out strings.Builder
	for _, line := range preserved {
		out.WriteString(line + "\n")
	}
	out.WriteString(managedBlock)

	tmp, err := os.CreateTemp(dir, ".authorized_keys-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(out.String()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}
