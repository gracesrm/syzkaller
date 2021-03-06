// Copyright 2015 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package prog

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
)

func TestClone(t *testing.T) {
	target, rs, iters := initTest(t)
	for i := 0; i < iters; i++ {
		p := target.Generate(rs, 10, nil)
		p1 := p.Clone()
		data := p.Serialize()
		data1 := p1.Serialize()
		if !bytes.Equal(data, data1) {
			t.Fatalf("program changed after clone\noriginal:\n%s\n\nnew:\n%s\n", data, data1)
		}
	}
}

func TestMutateRandom(t *testing.T) {
	testEachTargetRandom(t, func(t *testing.T, target *Target, rs rand.Source, iters int) {
	next:
		for i := 0; i < iters; i++ {
			p := target.Generate(rs, 10, nil)
			data0 := p.Serialize()
			p1 := p.Clone()
			// There is a chance that mutation will produce the same program.
			// So we check that at least 1 out of 20 mutations actually change the program.
			for try := 0; try < 20; try++ {
				p1.Mutate(rs, 10, nil, nil)
				data := p.Serialize()
				if !bytes.Equal(data0, data) {
					t.Fatalf("program changed after mutate\noriginal:\n%s\n\nnew:\n%s\n",
						data0, data)
				}
				data1 := p1.Serialize()
				if bytes.Equal(data, data1) {
					continue
				}
				if _, err := target.Deserialize(data1); err != nil {
					t.Fatalf("Deserialize failed after Mutate: %v\n%s", err, data1)
				}
				continue next
			}
			t.Fatalf("mutation does not change program:\n%s", data0)
		}
	})
}

func TestMutateCorpus(t *testing.T) {
	target, rs, iters := initTest(t)
	var corpus []*Prog
	for i := 0; i < 100; i++ {
		p := target.Generate(rs, 10, nil)
		corpus = append(corpus, p)
	}
	for i := 0; i < iters; i++ {
		p1 := target.Generate(rs, 10, nil)
		p1.Mutate(rs, 10, nil, corpus)
	}
}

func TestMutateTable(t *testing.T) {
	target := initTargetTest(t, "test", "64")
	tests := [][2]string{
		// Insert a call.
		{`
mutate0()
mutate2()
`, `
mutate0()
mutate1()
mutate2()
`},
		// Remove calls and update args.
		{`
r0 = mutate5(&(0x7f0000000000)="2e2f66696c653000", 0x0)
mutate0()
mutate6(r0, &(0x7f0000000000)="00", 0x1)
mutate1()
`, `
mutate0()
mutate6(0xffffffffffffffff, &(0x7f0000000000)="00", 0x1)
mutate1()
`},
		// Mutate flags.
		{`
r0 = mutate5(&(0x7f0000000000)="2e2f66696c653000", 0x0)
mutate0()
mutate6(r0, &(0x7f0000000000)="00", 0x1)
mutate1()
`, `
r0 = mutate5(&(0x7f0000000000)="2e2f66696c653000", 0xcdcdcdcdcdcdcdcd)
mutate0()
mutate6(r0, &(0x7f0000000000)="00", 0x1)
mutate1()
`},
		// Mutate data (delete byte and update size).
		{`
mutate4(&(0x7f0000000000)="11223344", 0x4)
`, `
mutate4(&(0x7f0000000000)="112244", 0x3)
`},
		// Mutate data (insert byte and update size).
		// TODO: this is not working, because Mutate constantly tends
		// update addresses and insert mmap's.
		/*
					{`
			mutate4(&(0x7f0000000000)="1122", 0x2)
			`, `
			mutate4(&(0x7f0000000000)="112200", 0x3)
			`},
		*/
		// Mutate data (change byte).
		{`
mutate4(&(0x7f0000000000)="1122", 0x2)
`, `
mutate4(&(0x7f0000000000)="1100", 0x2)
`},
		// Change filename.
		{`
mutate5(&(0x7f0000001000)="2e2f66696c653000", 0x22c0)
mutate5(&(0x7f0000001000)="2e2f66696c653000", 0x22c0)
`, `
mutate5(&(0x7f0000001000)="2e2f66696c653000", 0x22c0)
mutate5(&(0x7f0000001000)="2e2f66696c653100", 0x22c0)
`},
		// Extend an array.
		{`
mutate3(&(0x7f0000000000)=[0x1, 0x1], 0x2)
`, `
mmap(&(0x7f0000000000/0x1000)=nil, 0x1000)
mutate3(&(0x7f0000000000)=[0x1, 0x1, 0x1], 0x3)
`},
		// Mutate size from it's natural value.
		{`
mutate7(&(0x7f0000000000)='123', 0x3)
`, `
mutate7(&(0x7f0000000000)='123', 0x2)
`},
		// Mutate proc to the special value.
		{`
mutate8(0x2)
`, `
mutate8(0xffffffffffffffff)
`},
	}
	for ti, test := range tests {
		test := test
		t.Run(fmt.Sprint(ti), func(t *testing.T) {
			t.Parallel()
			p, err := target.Deserialize([]byte(test[0]))
			if err != nil {
				t.Fatalf("failed to deserialize original program: %v", err)
			}
			goal, err := target.Deserialize([]byte(test[1]))
			if err != nil {
				t.Fatalf("failed to deserialize goal program: %v", err)
			}
			want := goal.Serialize()
			enabled := make(map[*Syscall]bool)
			for _, c := range p.Calls {
				enabled[c.Meta] = true
			}
			for _, c := range goal.Calls {
				enabled[c.Meta] = true
			}
			ct := target.BuildChoiceTable(nil, enabled)
			rs := rand.NewSource(0)
			for i := 0; i < 1e5; i++ {
				p1 := p.Clone()
				p1.Mutate(rs, len(goal.Calls), ct, nil)
				data1 := p1.Serialize()
				if bytes.Equal(want, data1) {
					t.Logf("success on iter %v", i)
					return
				}
			}
			t.Fatalf("failed to achieve goal, original:%s\ngoal:%s", test[0], test[1])
		})
	}
}

func TestMinimize(t *testing.T) {
	tests := []struct {
		orig            string
		callIndex       int
		pred            func(*Prog, int) bool
		result          string
		resultCallIndex int
	}{
		// Predicate always returns false, so must get the same program.
		{
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"sched_yield()\n" +
				"pipe2(&(0x7f0000000000)={0x0, 0x0}, 0x0)\n",
			2,
			func(p *Prog, callIndex int) bool {
				if len(p.Calls) == 0 {
					t.Fatalf("got an empty program")
				}
				if p.Calls[len(p.Calls)-1].Meta.Name != "pipe2" {
					t.Fatalf("last call is removed")
				}
				return false
			},
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"sched_yield()\n" +
				"pipe2(&(0x7f0000000000), 0x0)\n",
			2,
		},
		// Remove a call.
		{
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"sched_yield()\n" +
				"pipe2(&(0x7f0000000000)={0xffffffffffffffff, 0xffffffffffffffff}, 0x0)\n",
			2,
			func(p *Prog, callIndex int) bool {
				// Aim at removal of sched_yield.
				return len(p.Calls) == 2 && p.Calls[0].Meta.Name == "mmap" && p.Calls[1].Meta.Name == "pipe2"
			},
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x0, 0x0, 0xffffffffffffffff, 0x0)\n" +
				"pipe2(&(0x7f0000000000)={0xffffffffffffffff, 0xffffffffffffffff}, 0x0)\n",
			1,
		},
		// Remove two dependent calls.
		{
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"pipe2(&(0x7f0000000000)={0x0, 0x0}, 0x0)\n" +
				"sched_yield()\n",
			2,
			func(p *Prog, callIndex int) bool {
				// Aim at removal of pipe2 and then mmap.
				if len(p.Calls) == 2 && p.Calls[0].Meta.Name == "mmap" && p.Calls[1].Meta.Name == "sched_yield" {
					return true
				}
				if len(p.Calls) == 1 && p.Calls[0].Meta.Name == "sched_yield" {
					return true
				}
				return false
			},
			"sched_yield()\n",
			0,
		},
		// Remove a call and replace results.
		{
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"pipe2(&(0x7f0000000000)={<r0=>0x0, 0x0}, 0x0)\n" +
				"write(r0, &(0x7f0000000000)=\"1155\", 0x2)\n" +
				"sched_yield()\n",
			3,
			func(p *Prog, callIndex int) bool {
				return p.String() == "mmap-write-sched_yield"
			},
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x0, 0x0, 0xffffffffffffffff, 0x0)\n" +
				"write(0xffffffffffffffff, &(0x7f0000000000), 0x0)\n" +
				"sched_yield()\n",
			2,
		},
		// Remove a call and replace results.
		{
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"r0=open(&(0x7f0000000000)=\"1155\", 0x0, 0x0)\n" +
				"write(r0, &(0x7f0000000000)=\"1155\", 0x2)\n" +
				"sched_yield()\n",
			-1,
			func(p *Prog, callIndex int) bool {
				return p.String() == "mmap-write-sched_yield"
			},
			"mmap(&(0x7f0000000000/0x1000)=nil, 0x1000, 0x0, 0x0, 0xffffffffffffffff, 0x0)\n" +
				"write(0xffffffffffffffff, &(0x7f0000000000), 0x0)\n" +
				"sched_yield()\n",
			-1,
		},
		// Glue several mmaps together.
		{
			"sched_yield()\n" +
				"mmap(&(0x7f0000010000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"mmap(&(0x7f0000011000/0x1000)=nil, 0x1000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n" +
				"getpid()\n" +
				"mmap(&(0x7f0000015000/0x5000)=nil, 0x2000, 0x3, 0x32, 0xffffffffffffffff, 0x0)\n",
			3,
			func(p *Prog, callIndex int) bool {
				return p.String() == "mmap-sched_yield-getpid"
			},
			"mmap(&(0x7f0000010000/0x7000)=nil, 0x7000, 0x0, 0x0, 0xffffffffffffffff, 0x0)\n" +
				"sched_yield()\n" +
				"getpid()\n",
			2,
		},
	}
	target, _, _ := initTest(t)
	for ti, test := range tests {
		p, err := target.Deserialize([]byte(test.orig))
		if err != nil {
			t.Fatalf("failed to deserialize original program #%v: %v", ti, err)
		}
		p1, ci := Minimize(p, test.callIndex, test.pred, false)
		res := p1.Serialize()
		if string(res) != test.result {
			t.Fatalf("minimization produced wrong result #%v\norig:\n%v\nexpect:\n%v\ngot:\n%v\n",
				ti, test.orig, test.result, string(res))
		}
		if ci != test.resultCallIndex {
			t.Fatalf("minimization broke call index #%v: got %v, want %v",
				ti, ci, test.resultCallIndex)
		}
	}
}

func TestMinimizeRandom(t *testing.T) {
	target, rs, iters := initTest(t)
	iters /= 10 // Long test.
	for i := 0; i < iters; i++ {
		p := target.Generate(rs, 5, nil)
		Minimize(p, len(p.Calls)-1, func(p1 *Prog, callIndex int) bool {
			return false
		}, true)
		Minimize(p, len(p.Calls)-1, func(p1 *Prog, callIndex int) bool {
			return true
		}, true)
	}
	for i := 0; i < iters; i++ {
		p := target.Generate(rs, 5, nil)
		Minimize(p, len(p.Calls)-1, func(p1 *Prog, callIndex int) bool {
			return false
		}, false)
		Minimize(p, len(p.Calls)-1, func(p1 *Prog, callIndex int) bool {
			return true
		}, false)
	}
}

func TestMinimizeCallIndex(t *testing.T) {
	target, rs, iters := initTest(t)
	r := rand.New(rs)
	for i := 0; i < iters; i++ {
		p := target.Generate(rs, 5, nil)
		ci := r.Intn(len(p.Calls))
		p1, ci1 := Minimize(p, ci, func(p1 *Prog, callIndex int) bool {
			return r.Intn(2) == 0
		}, r.Intn(2) == 0)
		if ci1 < 0 || ci1 >= len(p1.Calls) || p.Calls[ci].Meta.Name != p1.Calls[ci1].Meta.Name {
			t.Fatalf("bad call index after minimization")
		}
	}
}

func BenchmarkMutate(b *testing.B) {
	olddebug := debug
	debug = false
	defer func() { debug = olddebug }()
	target, err := GetTarget("linux", "amd64")
	if err != nil {
		b.Fatal(err)
	}
	prios := target.CalculatePriorities(nil)
	ct := target.BuildChoiceTable(prios, nil)
	const progLen = 30
	p := target.Generate(rand.NewSource(0), progLen, nil)
	b.RunParallel(func(pb *testing.PB) {
		rs := rand.NewSource(0)
		for pb.Next() {
			p.Clone().Mutate(rs, progLen, ct, nil)
		}
	})
}
