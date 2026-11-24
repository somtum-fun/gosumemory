// Package pp fills gosumemory's pp, star rating and map stats from somtum-pp: the pp the
// osu!somtum server pays, for every mode, with Relax (akatsuki osu_2019) and Autopilot. It
// replaces oppai-ng and gosu-pp, so the package (and the pp counters) need no cgo.
package pp

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/l3lackShark/gosumemory/memory"
	"github.com/somtum-fun/somtum-pp/go/somtumpp"
)

// Legacy mod bits read here.
const (
	modDoubleTime = 1 << 6
	modHalfTime   = 1 << 8
	modNightcore  = 1 << 9
)

type loadedMap struct {
	file  string
	mode  int32
	mods  uint32
	m     *somtumpp.Map
	times []int32 // hit object start times, ms
}

var current loadedMap

// GetData runs the pp loop. It does what pp.go, iffc.go, max.go and editor.go did before.
func GetData() {
	fmt.Println("[PP] " + somtumpp.Version())
	for {
		if memory.DynamicAddresses.IsReady {
			update()
		}
		time.Sleep(time.Duration(memory.UpdateTime) * time.Millisecond)
	}
}

// The old separate loops are folded into GetData; kept so callers still build.
func GetFCData()     {}
func GetMaxData()    {}
func GetEditorData() {}

func playing() bool { return memory.MenuData.OsuStatus == 2 }

func activeMode() int32 {
	if playing() {
		return memory.GameplayData.GameMode
	}
	return memory.MenuData.GameMode
}

func activeMods() uint32 { return uint32(memory.MenuData.Mods.AppliedMods) }

func update() {
	path := memory.MenuData.Bm.Path.FullDotOsu
	if !strings.HasSuffix(path, ".osu") {
		return
	}
	mode, mods := activeMode(), activeMods()
	if mode < 0 || mode > 3 {
		return
	}
	if current.file != memory.MenuData.Bm.Path.BeatmapOsuFileString || current.mode != mode || current.mods != mods || current.m == nil {
		if !load(path, mode, mods) {
			return
		}
	}

	switch memory.MenuData.OsuStatus {
	case 2: // playing
		live(mode, mods)
	case 1: // editor
		editor(mode, mods)
	case 5: // song select
		memory.GameplayData.PP.Pp = 0
	}
}

func load(path string, mode int32, mods uint32) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	m, err := somtumpp.ParseMap(b)
	if err != nil {
		memory.MenuData.PP.PpStrains = []float64{0}
		return false
	}
	if current.m != nil {
		current.m.Free()
	}
	current = loadedMap{file: memory.MenuData.Bm.Path.BeatmapOsuFileString, mode: mode, mods: mods, m: m, times: objectTimes(b)}
	if t, err := calculateMP3Time(); err == nil {
		memory.MenuData.Bm.Time.Mp3Time = t
	}

	base := somtumpp.NewScore(int(mode), mods)
	at := func(acc float64) int32 {
		s := base
		s.Accuracy = acc
		p, err := m.Calculate(s)
		if err != nil {
			return 0
		}
		return int32(math.Round(p.PP))
	}
	pps := &memory.MenuData.PP
	pps.PpSS, pps.Pp99, pps.Pp98, pps.Pp97, pps.Pp96, pps.Pp95 = at(100), at(99), at(98), at(97), at(96), at(95)

	ss := base
	ss.Accuracy = 100
	if p, err := m.Calculate(ss); err == nil {
		st := &memory.MenuData.Bm.Stats
		st.FullSR = round2(p.Stars)
		st.BeatmapSR = st.FullSR
		st.BeatmapMaxCombo = int32(p.MaxCombo)
	}
	if s, err := m.Stats(base); err == nil {
		st := &memory.MenuData.Bm.Stats
		st.BeatmapAR, st.BeatmapCS, st.BeatmapOD, st.BeatmapHP = round2(s.AR), round2(s.CS), round2(s.OD), round2(s.HP)
	}
	if v, _, err := m.Strains(base); err == nil && len(v) > 0 {
		memory.MenuData.PP.PpStrains = v
	} else {
		memory.MenuData.PP.PpStrains = []float64{0}
	}
	min, max := bpmRange(b, mods)
	memory.MenuData.Bm.Stats.BeatmapBPM.Minimal, memory.MenuData.Bm.Stats.BeatmapBPM.Maximal = min, max
	if n := len(current.times); n > 0 {
		memory.MenuData.Bm.Time.FirstObj = current.times[0]
		memory.MenuData.Bm.Time.FullTime = current.times[n-1]
	}
	return true
}

// judged is how many objects a stable play has judged, from its hit counts.
func judged(mode int32, h300, h100, h50, h0, geki, katu int) int {
	switch mode {
	case 1, 2: // catch: fruits + droplets + misses; tiny droplets (50/katu) aren't objects
		return h300 + h100 + h0
	case 3:
		return geki + h300 + katu + h100 + h50 + h0
	default:
		return h300 + h100 + h50 + h0
	}
}

func live(mode int32, mods uint32) {
	g := &memory.GameplayData
	h300, h100, h50, h0 := int(g.Hits.H300), int(g.Hits.H100), int(g.Hits.H50), int(g.Hits.H0)
	geki, katu := int(g.Hits.HGeki), int(g.Hits.HKatu)
	done := judged(mode, h300, h100, h50, h0, geki, katu)
	if done == 0 {
		g.PP.Pp = 0
		return
	}
	n := current.m.NObjects()
	s := somtumpp.NewScore(int(mode), mods)
	s.N300, s.N100, s.N50, s.NMiss, s.NGeki, s.NKatu = h300, h100, h50, h0, geki, katu
	s.MaxCombo = int(g.Combo.Max)
	if done < n {
		s.PassedObjects = done
	}
	if p, err := current.m.Calculate(s); err == nil {
		g.PP.Pp = int32(math.Round(p.PP))
		memory.MenuData.Bm.Stats.BeatmapSR = round2(p.Stars)
	}

	remaining := n - done
	if remaining < 0 {
		remaining = 0
	}
	// If FC: the misses as 300s and the rest of the map perfect, full combo.
	fc := somtumpp.NewScore(int(mode), mods)
	fc.N300, fc.N100, fc.N50, fc.NGeki, fc.NKatu = h300+h0+remaining, h100, h50, geki, katu
	if p, err := current.m.Calculate(fc); err == nil {
		g.PP.PPifFC = int32(math.Round(p.PP))
	}
	// Max this play: the rest of the map perfect, misses kept.
	mx := somtumpp.NewScore(int(mode), mods)
	mx.N300, mx.N100, mx.N50, mx.NMiss, mx.NGeki, mx.NKatu = h300+remaining, h100, h50, h0, geki, katu
	mx.MaxCombo = int(math.Max(float64(g.Combo.Max), float64(int(g.Combo.Current)+remaining)))
	if p, err := current.m.Calculate(mx); err == nil {
		g.PP.PPMaxThisPlay = int32(math.Round(p.PP))
	}

	g.Hits.Grade.Current = calculateGrade(float32(h300), float32(h100), float32(h50), float32(h0))
	g.Hits.Grade.Expected = calculateGrade(float32(h300+remaining), float32(h100), float32(h50), float32(h0))
}

// editor: pp of the map up to the editor's position, played perfectly.
func editor(mode int32, mods uint32) {
	t := memory.MenuData.Bm.Time.PlayTime
	passed := 0
	for _, ot := range current.times {
		if ot <= t {
			passed++
		}
	}
	if passed == 0 {
		memory.GameplayData.PP.Pp = 0
		return
	}
	s := somtumpp.NewScore(int(mode), mods)
	s.Accuracy = 100
	if passed < current.m.NObjects() {
		s.PassedObjects = passed
	}
	if p, err := current.m.Calculate(s); err == nil {
		memory.GameplayData.PP.Pp = int32(math.Round(p.PP))
		memory.MenuData.Bm.Stats.BeatmapSR = round2(p.Stars)
	}
}

func round2(v float64) float32 {
	return float32(math.Round(v*100) / 100)
}

// objectTimes reads each hit object's start time from the .osu file.
func objectTimes(osu []byte) []int32 {
	var out []int32
	sc := bufio.NewScanner(bytes.NewReader(osu))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	in := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			in = line == "[HitObjects]"
			continue
		}
		if !in || line == "" {
			continue
		}
		f := strings.SplitN(line, ",", 4)
		if len(f) < 3 {
			continue
		}
		if t, err := strconv.ParseFloat(f[2], 64); err == nil {
			out = append(out, int32(t))
		}
	}
	return out
}

// bpmRange is the map's lowest and highest BPM (uninherited timing points), at the play's speed.
func bpmRange(osu []byte, mods uint32) (int, int) {
	rate := 1.0
	if mods&(modDoubleTime|modNightcore) != 0 {
		rate = 1.5
	} else if mods&modHalfTime != 0 {
		rate = 0.75
	}
	var bpms []int
	sc := bufio.NewScanner(bytes.NewReader(osu))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	in := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			in = line == "[TimingPoints]"
			continue
		}
		if !in || line == "" {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) < 2 {
			continue
		}
		beat, err := strconv.ParseFloat(f[1], 64)
		if err != nil || beat <= 0 {
			continue // inherited (negative) or broken
		}
		if len(f) >= 7 && strings.TrimSpace(f[6]) == "0" {
			continue
		}
		if bpm := int(math.Round(60000 / beat * rate)); bpm > 0 {
			bpms = append(bpms, bpm)
		}
	}
	return minMax(bpms)
}
