// Command doccheck checks a small set of canonical Markdown entry files.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var links=regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
func main(){for _,p:=range []string{"README.md","AGENTS.md","docs/index.md","docs/architecture.md","docs/evidence/template.md"}{b,e:=os.ReadFile(p);if e!=nil {fatal(e)};if e=check(p,string(b));e!=nil {fatal(e)}};fmt.Println("local docs links: PASS")}
func fatal(err error){fmt.Fprintln(os.Stderr,err);os.Exit(1)}
func check(path,contents string)error{
	for _,hit:=range links.FindAllStringSubmatch(contents,-1){
		target:=hit[1];if strings.HasPrefix(target,"http:")||strings.HasPrefix(target,"https:")||strings.HasPrefix(target,"mailto:")||strings.HasPrefix(target,"#"){continue}
		target=strings.SplitN(target,"#",2)[0];if target==""{continue}
		if strings.HasPrefix(target,"/"){return fmt.Errorf("absolute local link %s",target)}
		dest:=filepath.Clean(filepath.Join(filepath.Dir(path),target))
		if dest==".."||strings.HasPrefix(dest,".."+string(filepath.Separator)){return fmt.Errorf("repo escape: %s",target)}
		if _,err:=os.Stat(dest);err!=nil{return fmt.Errorf("%s: %w",target,err)}
	}
	return nil
}
