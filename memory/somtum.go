package memory

// somtum.fun: support for Somtum.Client (somtum-stable.exe), osu!stable recompiled for
// somtum.fun (stable40 and cuttingedge). It is the same game underneath; three things differ:
// the exe name, two signatures whose code JITs differently (same targets), and where the game
// mode static sits next to Base. Checked on b20261118.1 stable40 and b20261118 cuttingedge.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/l3lackShark/gosumemory/mem"
)

const somtumStableExe = "somtum-stable.exe"

// Base minus this = the operand holding Player.mode's address (the ruleset; RetryCount +0x8,
// PlayCount +0xc). osu!stable: 0x33. Somtum.Client: 0x2D (the `cmp [Player.mode],1` operand;
// 0x33 back is inside `cmp [GameBase.Mode],2` there, the screen state).
const (
	stablePlayerModeOffset = 0x33
	somtumPlayerModeOffset = 0x2D
)

// somtumAddresses are the signatures that differ in Somtum.Client. Each is turned into the
// address the stable expression expects (see applySomtumAddresses), so the readers stay as is.
type somtumAddresses struct {
	// ret; mov eax,[esi+4]; mov edx,[AudioEngine.Time]; mov [eax+4],edx
	PlayTime int64 `sig:"C3 8B 46 04 8B 15 ?? ?? ?? ?? 89 50 04"`
	// je; test edi,edi; jne; mov esi,[SkinManager.Current]; mov eax,[SkinManager.CurrentUserSkin]
	SkinData int64 `sig:"74 ?? 85 FF 75 ?? 8B 35 ?? ?? ?? ?? A1"`
}

func isSomtumStable(p mem.Process) bool {
	exe, err := p.ExecutablePath()
	if err == nil && strings.EqualFold(filepath.Base(exe), somtumStableExe) {
		return true
	}
	// Linux/Wine: the executable is wine-preloader; the Windows path is the first argument.
	if cmd, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(p.Pid()), "cmdline")); err == nil {
		return strings.Contains(strings.ToLower(string(cmd)), somtumStableExe)
	}
	return false
}

// resolveAllPatterns resolves the stable signatures, then fills what Somtum.Client needs.
func resolveAllPatterns(p mem.Process, addrs *staticAddresses) error {
	err := mem.ResolvePatterns(p, addrs)
	addrs.PlayerMode = addrs.Base - stablePlayerModeOffset
	if !isSomtumStable(p) {
		return err
	}
	var s somtumAddresses
	if serr := mem.ResolvePatterns(p, &s); serr != nil {
		return serr
	}
	// [PlayTime + 0x5] = AudioEngine.Time: the operand is 6 bytes into our signature.
	addrs.PlayTime = s.PlayTime + 1
	// [[[SkinData + 4] + 0] + 68]: the operand is 8 bytes into our signature.
	addrs.SkinData = s.SkinData + 4
	addrs.PlayerMode = addrs.Base - somtumPlayerModeOffset
	// Only the two signatures above differ in Somtum.Client; anything else missing is real.
	if addrs.Base == 0 || addrs.MenuMods == 0 || addrs.Rulesets == 0 || addrs.ChatChecker == 0 {
		return err
	}
	return nil
}

// somtumDataDir is where Somtum.Client keeps the player's osu! data when the exe lives elsewhere,
// found as the client finds it: SOMTUM_OSU_DIR in its environment (the launcher sets it), then
// "osu_dir" in somtum-stable.json beside the exe. Empty when the exe sits in the osu! folder.
func somtumDataDir(p mem.Process, exeDir string) string {
	if dir := mem.ProcessEnv(p, "SOMTUM_OSU_DIR"); isOsuDir(dir) {
		return dir
	}
	b, err := os.ReadFile(filepath.Join(exeDir, "somtum-stable.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		OsuDir string `json:"osu_dir"`
	}
	if json.Unmarshal(b, &cfg) == nil && isOsuDir(cfg.OsuDir) {
		return cfg.OsuDir
	}
	return ""
}

func isOsuDir(dir string) bool {
	if dir == "" {
		return false
	}
	for _, f := range []string{"osu!.db", "Songs"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

// osuProcessRegex matches osu!stable and Somtum.Client.
var osuProcessRegex = regexp.MustCompile(`(?i).*(osu!|somtum-stable)\.exe.*`)

// pinnedProcess keeps only the process GOSUMEMORY_PID names, when set: with two clients open
// (say osu!stable and Somtum.Client) gosumemory would otherwise assume a tournament.
func pinnedProcess(procs []mem.Process) []mem.Process {
	pid, err := strconv.Atoi(os.Getenv("GOSUMEMORY_PID"))
	if err != nil {
		return procs
	}
	for _, p := range procs {
		if p.Pid() == pid {
			return []mem.Process{p}
		}
	}
	return procs
}
