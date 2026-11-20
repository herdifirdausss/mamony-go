package domain

import (
	"errors"
	"math"
	"testing"
)

func TestValidateJournal(t *testing.T) {
	valid := []Entry{{"a","IDR",Debit,100},{"b","IDR",Credit,100}}
	cases := []struct{name string; entries []Entry; good bool}{
		{"balanced",valid,true},
		{"empty",nil,false},
		{"unbalanced",[]Entry{{"a","IDR",Debit,100},{"b","IDR",Credit,99}},false},
		{"mixed",[]Entry{{"a","USD",Debit,100},{"b","IDR",Credit,100}},false},
		{"negative",[]Entry{{"a","IDR",Debit,-1},{"b","IDR",Credit,-1}},false},
		{"zero",[]Entry{{"a","IDR",Debit,0},{"b","IDR",Credit,0}},false},
		{"bad side",[]Entry{{"a","IDR","other",100},{"b","IDR",Credit,100}},false},
		{"overflow",[]Entry{{"a","IDR",Debit,math.MaxInt64},{"a","IDR",Debit,1},{"b","IDR",Credit,1}},false},
		{"multi",[]Entry{{"a","IDR",Debit,40},{"a","IDR",Debit,60},{"b","IDR",Credit,100}},true},
	}
	for _,tc:= range cases {t.Run(tc.name,func(t *testing.T){ err:=ValidateJournal(tc.entries);if tc.good && err!=nil {t.Fatal(err)};if !tc.good && !errors.Is(err,ErrInvalidJournal){t.Fatalf("expected ErrInvalidJournal: %v",err)} })}
}
