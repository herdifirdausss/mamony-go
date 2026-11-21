package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestCanonicalEmail(t *testing.T) {
	good := map[string]string{
		"ALICE@EXAMPLE.COM":"alice@example.com",
		"user+tag@example.co.id":"user+tag@example.co.id",
	}
	for input,want := range good {
		got,err:=CanonicalEmail(input)
		if err!=nil || got!=want {t.Fatalf("%q: got %q, %v",input,got,err)}
	}
	for _,input:=range []string{""," a@example.com","alice@","@example.com","a@@example.com","alice@localhost","a..b@example.com","a@-example.com","a@ex ample.com","ußer@example.com","a@example.com\n"} {
		if _,err:=CanonicalEmail(input); !errors.Is(err,ErrInvalidEmail) {t.Errorf("%q unexpectedly accepted: %v",input,err)}
	}
}

func TestPasswordPolicyNFCAndBoundaries(t *testing.T) {
	p:=PasswordPolicy{IsBlocked:func(p,e string)bool{return p=="known-compromised-string" || strings.Contains(p,e)}}
	normal:="e\u0301"+strings.Repeat("different-syllables",2)
	got,err:=p.NormalizeAndValidate(normal,"alice@example.com")
	if err!=nil || !strings.HasPrefix(got,"é"){t.Fatalf("NFC policy: %q %v",got,err)}
	for _,password:=range []string{"short","known-compromised-string",strings.Repeat("a",129),strings.Repeat("界",129),"wrong\xffvalue","alice@example.com is secret"} {
		if _,err:=p.NormalizeAndValidate(password,"alice@example.com");err==nil{t.Errorf("invalid password accepted")}
	}
	if _,err:=PasswordPolicy{}.NormalizeAndValidate(strings.Repeat("x",20),"");!errors.Is(err,ErrInvalidPassword){t.Fatal("missing blocklist must fail closed")}
	if _,err:=p.NormalizeAndValidate("  "+strings.Repeat("a",15)+"  ","");err!=nil{t.Fatalf("surrounding spaces must remain accepted: %v",err)}
}
