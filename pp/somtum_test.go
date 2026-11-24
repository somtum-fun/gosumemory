package pp

import (
	"testing"

	"github.com/somtum-fun/somtum-pp/go/somtumpp"
)

const osuFile = `osu file format v14

[TimingPoints]
1000,500,4,2,0,60,1,0
2000,-100,4,2,0,60,0,0
3000,250,4,2,0,60,1,0

[HitObjects]
256,192,1000,1,0,0:0:0:0:
256,192,1500,1,0,0:0:0:0:
256,192,4000,1,0,0:0:0:0:
`

func TestObjectTimes(t *testing.T) {
	got := objectTimes([]byte(osuFile))
	if len(got) != 3 || got[0] != 1000 || got[2] != 4000 {
		t.Fatalf("objectTimes = %v", got)
	}
}

func TestBPMRange(t *testing.T) {
	if lo, hi := bpmRange([]byte(osuFile), 0); lo != 120 || hi != 240 {
		t.Fatalf("NM bpm %d-%d", lo, hi)
	}
	if lo, hi := bpmRange([]byte(osuFile), modDoubleTime); lo != 180 || hi != 360 {
		t.Fatalf("DT bpm %d-%d", lo, hi)
	}
}

func TestJudged(t *testing.T) {
	if judged(0, 10, 2, 1, 1, 0, 0) != 14 || judged(3, 1, 1, 1, 1, 1, 1) != 6 || judged(2, 5, 3, 9, 1, 0, 4) != 9 {
		t.Fatal("judged")
	}
}

func TestSomtumPP(t *testing.T) {
	if somtumpp.Route(0, somtumpp.Relax) != somtumpp.Relax2019 {
		t.Fatal("relax route")
	}
}
