//go:build linux

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/ASentimentalMan/NAS-TikTok/server/internal/logging"
	"golang.org/x/sys/unix"
)

const serverPath = "/usr/local/bin/nas-tiktok"

func main() {
	runtime.LockOSThread()
	logger := logging.FromEnv()
	if err := clearAmbientCapabilities(); err != nil {
		logger.Errorf("ambient capability cleanup failed; skipping ownership initialization: %v", err)
	} else {
		if err := enableChownFromPermitted(); err != nil {
			logger.Errorf("capability initialization failed; skipping ownership initialization: %v", err)
		} else {
			changed, err := prepareWorkspace("/workspace", os.Getuid(), os.Getgid())
			if err != nil {
				logger.Errorf("workspace ownership initialization failed; server will use HDD fallback where needed: %v", err)
			} else if changed {
				logger.Infof("workspace ownership initialized for uid:gid %d:%d", os.Getuid(), os.Getgid())
			}
		}
	}
	if err := execServer(os.Args[1:], os.Environ()); err != nil {
		logger.Errorf("server startup failed: %v", err)
		os.Exit(1)
	}
}

// enableChownFromPermitted activates no capability other than CAP_CHOWN. The
// helper can then fix existing ownership without receiving ambient privileges.
func enableChownFromPermitted() error {
	var header unix.CapUserHeader
	header.Version = unix.LINUX_CAPABILITY_VERSION_3
	var data [2]unix.CapUserData
	if err := unix.Capget(&header, &data[0]); err != nil {
		return fmt.Errorf("read capabilities: %w", err)
	}
	bit := uint32(1 << unix.CAP_CHOWN)
	if data[0].Permitted&bit == 0 {
		data = [2]unix.CapUserData{}
		if err := unix.Capset(&header, &data[0]); err != nil {
			return fmt.Errorf("clear capabilities without CAP_CHOWN: %w", err)
		}
		return nil
	}
	data = [2]unix.CapUserData{{Effective: bit, Permitted: bit}, {}}
	if err := unix.Capset(&header, &data[0]); err != nil {
		return fmt.Errorf("restrict and activate CAP_CHOWN: %w", err)
	}
	return nil
}

func execServer(args, env []string) error {
	if err := clearAmbientCapabilities(); err != nil {
		return fmt.Errorf("clear ambient capabilities: %w", err)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("set no_new_privs: %w", err)
	}
	var header unix.CapUserHeader
	header.Version = unix.LINUX_CAPABILITY_VERSION_3
	var noCapabilities [2]unix.CapUserData
	if err := unix.Capset(&header, &noCapabilities[0]); err != nil {
		return fmt.Errorf("drop process capabilities: %w", err)
	}
	argv := make([]string, 1, len(args)+1)
	argv[0] = serverPath
	argv = append(argv, args...)
	if err := unix.Exec(serverPath, argv, env); err != nil {
		return fmt.Errorf("exec %s: %w", serverPath, err)
	}
	return nil
}

func clearAmbientCapabilities() error {
	err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0)
	if err == unix.EINVAL || err == unix.ENOSYS {
		return nil
	}
	return err
}
