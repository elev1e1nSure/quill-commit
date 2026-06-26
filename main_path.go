package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	hwndBroadcast   = 0xFFFF
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procSendMessageTO  = user32.NewProc("SendMessageTimeoutW")
)

func handlePath(action string) error {
	if action != "add" && action != "remove" {
		return fmt.Errorf("--path expects 'add' or 'remove', got %q", action)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("cannot open HKCU\\Environment: %w", err)
	}
	defer key.Close()

	currentPath, _, err := key.GetStringValue("PATH")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("cannot read PATH from registry: %w", err)
	}

	entries := splitPath(currentPath)
	var newPath string

	switch action {
	case "add":
		if contains(entries, exeDir) {
			fmt.Printf("%s is already in PATH\n", exeDir)
			return nil
		}
		entries = append(entries, exeDir)
		newPath = strings.Join(entries, string(os.PathListSeparator))

		if err := key.SetStringValue("PATH", newPath); err != nil {
			return fmt.Errorf("cannot write PATH to registry: %w", err)
		}

		fmt.Printf("Added %s to user PATH\n", exeDir)

	case "remove":
		filtered := make([]string, 0, len(entries))
		for _, e := range entries {
			if !strings.EqualFold(e, exeDir) {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == len(entries) {
			fmt.Printf("%s was not in PATH\n", exeDir)
			return nil
		}
		newPath = strings.Join(filtered, string(os.PathListSeparator))

		if err := key.SetStringValue("PATH", newPath); err != nil {
			return fmt.Errorf("cannot write PATH to registry: %w", err)
		}

		fmt.Printf("Removed %s from user PATH\n", exeDir)
	}

	broadcastEnvChange()
	os.Setenv("PATH", newPath)

	return nil
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	raw := strings.Split(p, string(os.PathListSeparator))
	entries := make([]string, 0, len(raw))
	for _, e := range raw {
		e = strings.TrimSpace(e)
		if e != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}

func broadcastEnvChange() {
	envPtr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	//nolint:errcheck
	procSendMessageTO.Call(
		hwndBroadcast,
		wmSettingChange,
		uintptr(0),
		uintptr(unsafe.Pointer(envPtr)),
		smtoAbortIfHung,
		uintptr(5000),
		uintptr(0),
	)
}
