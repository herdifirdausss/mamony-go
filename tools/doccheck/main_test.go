package main
import "testing"
func TestLinks(t *testing.T){
	if e:=check("main_test.go","[source](main.go)");e!=nil {t.Fatal(e)}
	if e:=check("docs/index.md","[escape](../../etc/passwd)");e==nil {t.Fatal("path escape allowed")}
	if e:=check("README.md","[missing](no-such-file.md)");e==nil {t.Fatal("broken link allowed")}
}
