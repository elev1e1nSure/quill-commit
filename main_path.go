package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"quill-commit/credentials"
)

const (
	hwndBroadcast   = 0xFFFF
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	procSendMessageTO = user32.NewProc("SendMessageTimeoutW")
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
			fmt.Println(stFlag.Render(exeDir) + " " + stMeta.Render("is already in PATH"))
			return nil
		}
		entries = append(entries, exeDir)
		newPath = strings.Join(entries, string(os.PathListSeparator))

		if err := key.SetStringValue("PATH", newPath); err != nil {
			return fmt.Errorf("cannot write PATH to registry: %w", err)
		}

		fmt.Println(stTitle.Render("Added") + " " + stFlag.Render(exeDir) + " " + stMeta.Render("to user PATH"))
		defer printPathRefresh(exeDir)

	case "remove":
		filtered := make([]string, 0, len(entries))
		for _, e := range entries {
			if !strings.EqualFold(e, exeDir) {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == len(entries) {
			fmt.Println(stFlag.Render(exeDir) + " " + stMeta.Render("was not in PATH"))
			return nil
		}
		newPath = strings.Join(filtered, string(os.PathListSeparator))

		if err := key.SetStringValue("PATH", newPath); err != nil {
			return fmt.Errorf("cannot write PATH to registry: %w", err)
		}

		fmt.Println(stTitle.Render("Removed") + " " + stFlag.Render(exeDir) + " " + stMeta.Render("from user PATH"))
	}

	broadcastEnvChange()
	os.Setenv("PATH", newPath)

	return nil
}

// ensurePathOnFirstRun silently adds the executable's directory to the user
// PATH the very first time quill-commit ever runs, so it can be invoked from
// any shell afterwards. Guarded by a marker file — best-effort, never blocks
// startup on failure.
func ensurePathOnFirstRun() {
	marker, err := firstRunMarkerPath()
	if err != nil {
		return
	}
	if _, err := os.Stat(marker); err == nil {
		return
	}

	if err := addExeToPathSilently(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", stWarn.Render("warn:"), err)
	}

	if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
		return
	}
	_ = os.WriteFile(marker, []byte("1"), 0600)
}

func firstRunMarkerPath() (string, error) {
	dir, err := credentials.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "path-initialized"), nil
}

func addExeToPathSilently() error {
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
	if contains(entries, exeDir) {
		return nil
	}
	entries = append(entries, exeDir)
	newPath := strings.Join(entries, string(os.PathListSeparator))

	if err := key.SetStringValue("PATH", newPath); err != nil {
		return fmt.Errorf("cannot write PATH to registry: %w", err)
	}

	broadcastEnvChange()
	os.Setenv("PATH", newPath)

	fmt.Println(stTitle.Render("Added") + " " + stFlag.Render(exeDir) + " " + stMeta.Render("to user PATH (first run)"))
	printPathRefresh(exeDir)

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

func printPathRefresh(dir string) {
	fmt.Println()
	fmt.Println(stMeta.Render("New terminals will pick up the PATH automatically."))
	fmt.Println(stMeta.Render("To use quill-commit in this terminal, run:"))
	fmt.Println()
	fmt.Println("  " + stFlag.Render("$env:Path") + stMeta.Render(" += ") + stFlag.Render("\";"+dir+"\""))
}
