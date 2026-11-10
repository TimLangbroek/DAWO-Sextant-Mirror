package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/joincode"
)

func newJoinCode(id string, uses int, created time.Time) joincode.Code {
	return joincode.Code{
		ID: id, Hash: joincode.Hash(joincode.Key([]byte("k")), id),
		Group: "pilot", Uses: uses, Created: created, CreatedBy: "beheerder",
		Expires: created.Add(joincode.TTL),
	}
}

func TestJoinCodeSpendCountsDownAndCloses(t *testing.T) {
	s := openStore(t).JoinCodes()
	ctx := context.Background()
	c := newJoinCode("c1", 2, t0)
	if err := s.Create(ctx, "t1", c); err != nil {
		t.Fatal(err)
	}
	for i, wantBefore := range []int{2, 1} {
		got, ok, err := s.Spend(ctx, "t1", c.Hash, t0.Add(time.Hour))
		if err != nil || !ok {
			t.Fatalf("spend %d: ok=%v err=%v", i, ok, err)
		}
		if got.ID != "c1" || got.Group != "pilot" || got.Uses != wantBefore || got.CreatedBy != "beheerder" {
			t.Fatalf("spend %d returned %+v", i, got)
		}
	}
	if _, ok, err := s.Spend(ctx, "t1", c.Hash, t0.Add(time.Hour)); err != nil || ok {
		t.Fatalf("a spent code was spent again: ok=%v err=%v", ok, err)
	}
	if open, _ := s.Open(ctx, "t1", t0.Add(time.Hour)); len(open) != 0 {
		t.Fatalf("a spent code is listed as open: %+v", open)
	}

	// A failed join gives its use back.
	if err := s.Refund(ctx, "t1", "c1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Spend(ctx, "t1", c.Hash, t0.Add(time.Hour)); !ok {
		t.Fatal("a refunded use cannot be spent")
	}
}

func TestJoinCodeRefusesExpiredUnknownAndOtherTenants(t *testing.T) {
	s := openStore(t).JoinCodes()
	ctx := context.Background()
	c := newJoinCode("c1", 1, t0)
	if err := s.Create(ctx, "t1", c); err != nil {
		t.Fatal(err)
	}
	for name, try := range map[string]func() (bool, error){
		"at expiry": func() (bool, error) {
			_, ok, err := s.Spend(ctx, "t1", c.Hash, c.Expires)
			return ok, err
		},
		"unknown": func() (bool, error) {
			_, ok, err := s.Spend(ctx, "t1", []byte("nope"), t0)
			return ok, err
		},
		"another tenant": func() (bool, error) {
			_, ok, err := s.Spend(ctx, "t2", c.Hash, t0)
			return ok, err
		},
	} {
		if ok, err := try(); err != nil || ok {
			t.Errorf("%s: spent (ok=%v err=%v)", name, ok, err)
		}
	}
	// None of those spent the one use.
	if _, ok, _ := s.Spend(ctx, "t1", c.Hash, t0); !ok {
		t.Fatal("refused attempts used up the code")
	}
}

func TestJoinCodeLastUseGoesToOneLaptop(t *testing.T) {
	s := openStore(t).JoinCodes()
	ctx := context.Background()
	c := newJoinCode("c1", 1, t0)
	if err := s.Create(ctx, "t1", c); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := s.Spend(ctx, "t1", c.Hash, t0)
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("the last use of a code went to %d laptops", won)
	}
}

func TestJoinCodeOpenWithdrawAndPrune(t *testing.T) {
	s := openStore(t).JoinCodes()
	ctx := context.Background()
	old := newJoinCode("old", 1, t0)
	a := newJoinCode("a", 1, t0.Add(9*24*time.Hour))
	b := newJoinCode("b", 3, a.Created.Add(time.Minute))
	for _, c := range []joincode.Code{old, a, b} {
		if err := s.Create(ctx, "t1", c); err != nil {
			t.Fatal(err)
		}
	}
	now := b.Created.Add(time.Minute)
	open, err := s.Open(ctx, "t1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != "b" || open[1].ID != "a" {
		t.Fatalf("open codes, newest first: %+v", open)
	}
	if err := s.Withdraw(ctx, "t1", "b"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Spend(ctx, "t1", b.Hash, now); ok {
		t.Fatal("a withdrawn code was spent")
	}
	// The code that expired more than a week before "a" was made is gone:
	// refunding it changes nothing and listing at its own time shows nothing.
	if err := s.Refund(ctx, "t1", "old"); err != nil {
		t.Fatal(err)
	}
	open, _ = s.Open(ctx, "t1", t0)
	for _, c := range open {
		if c.ID == "old" {
			t.Fatalf("a code closed for over a week was not pruned: %+v", c)
		}
	}
}
