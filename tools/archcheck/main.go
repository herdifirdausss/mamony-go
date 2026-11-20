// Command archcheck enforces capability import boundaries.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const module = "github.com/herdifirdausss/mamony-go"
type pkg struct { ImportPath string; Imports,TestImports,XTestImports []string; Error *struct{Err string} }

func main() {
	cmd := exec.Command("go","list","-json","./...")
	cmd.Stderr=os.Stderr
	out,err:=cmd.StdoutPipe();if err!=nil {fatal(err)}
	if err=cmd.Start();err!=nil {fatal(err)}
	dec:=json.NewDecoder(out)
	n:=0
	for {var p pkg;err:=dec.Decode(&p);if err==io.EOF {break};if err!=nil {fatal(err)}
		if p.Error!=nil {fatal(fmt.Errorf("%s: %s",p.ImportPath,p.Error.Err))}
		for _,dep:=range append(append(p.Imports,p.TestImports...),p.XTestImports...) {if reason:=violation(p.ImportPath,dep);reason!="" {fmt.Fprintf(os.Stderr,"ARCH VIOLATION: %s -> %s: %s\n",p.ImportPath,dep,reason);n++}}
	}
	if err=cmd.Wait();err!=nil {fatal(err)}
	if n!=0 {fmt.Fprintf(os.Stderr,"%d architecture violations\n",n);os.Exit(1)}
	fmt.Println("architecture import policy: PASS")
}
func fatal(e error){fmt.Fprintln(os.Stderr,e);os.Exit(1)}
func local(p string)string {prefix:=module+"/";if strings.HasPrefix(p,prefix){return strings.TrimPrefix(p,prefix)};return ""}
func layer(p string)(string,string){parts:=strings.Split(p,"/");if len(parts)<3||parts[0]!="internal"||parts[1]!="modules"{return "",""};if len(parts)>4 && parts[3]=="internal" {return parts[2],parts[4]};return parts[2],""}
func violation(from,to string)string {
	a,b:=local(from),local(to);if a==""||b==""{return ""}
	ca,la:=layer(a);cb,lb:=layer(b)
	if ca!="" && cb!="" && ca!=cb {return "cross-capability import"}
	if strings.HasPrefix(a,"internal/platform/") && (cb!=""||strings.HasPrefix(b,"internal/app")) {return "platform must not import app/modules"}
	if ca!="" && strings.HasPrefix(b,"internal/app") {return "module must not import app"}
	if ca!="" && la=="domain" && (lb=="usecase"||lb=="repository"||lb=="handler"||strings.HasPrefix(b,"internal/platform/")) {return "domain is pure"}
	if ca!="" && la=="usecase" && (lb=="repository"||lb=="handler") {return "usecase must not import adapters"}
	if ca!="" && la=="handler" && lb=="repository" {return "handler must not import repository"}
	if cb!="" && lb!="" && strings.HasPrefix(a,"internal/app") {return "app must use module facade"}
	return ""
}
