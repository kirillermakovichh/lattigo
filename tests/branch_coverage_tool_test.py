"""Check instrumentation semantics and protect the working checkout/output files."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class BranchInstrumentation(unittest.TestCase):
    def test_control_flow_semantics_and_decision_outcomes(self):
        with tempfile.TemporaryDirectory(prefix="bl-branch-tool-test-") as temporary:
            copy = Path(temporary) / "copy"
            sample = copy / "internal/sample"
            sample.mkdir(parents=True)
            (copy / "cmd").mkdir()
            (copy / "go.mod").write_text("module example.com/go-block-lattice\n\ngo 1.26.0\n")
            (sample / "sample.go").write_text('''package sample
 type Flag bool
 func Decide(flag Flag,n int,items []int,value any,ch <-chan int)int{
  score:=0
  if flag{score++}else{score--}
  for i:=0;i<n;i++{score++}
  for _,item:=range items{if item<0{return score};score+=item}
  switch n{case 0:score+=10;case 1:score+=20}
  switch value.(type){case string:score+=2;default:score+=3}
  select{case <-ch:score++;default:score-=2}
  return score
 }
 func Sum(groups [][]int)int{
  total:=0
 Outer:
  for _,group:=range groups{
   for _,value:=range group{if value<0{continue Outer};if value==0{break};total+=value}
  }
  return total
 }
''')
            (sample / "sample_test.go").write_text('''package sample
 import "testing"
 func TestDecisions(t *testing.T){
  ready:=make(chan int,1);ready<-1
  if Decide(false,0,nil,nil,nil)!=10{t.Fatal("empty case")}
  if Decide(true,1,[]int{1},"text",ready)!=26{t.Fatal("ready case")}
  if Decide(true,2,[]int{1},1,nil)!=5{t.Fatal("switch default")}
  if Decide(true,0,[]int{-1},nil,nil)!=1{t.Fatal("early return")}
  if Sum(nil)!=0 || Sum([][]int{{1,-1},nil,{0}})!=1{t.Fatal("labeled range")}
 }
''')
            inventory = Path(temporary) / "inventory.json"
            hits = Path(temporary) / "hits"
            hits.mkdir()
            env = dict(os.environ, GOCACHE=str(ROOT / ".cache/go-build"), BL_BRANCH_OUT=str(hits))
            subprocess.run(["go", "run", str(ROOT / "scripts/branch_coverage/instrument.go"),
                            str(copy), str(inventory)], cwd=ROOT, env=env, check=True,
                           capture_output=True, text=True)
            subprocess.run(["go", "test", "./..."], cwd=copy, env=env, check=True,
                           capture_output=True, text=True)
            observed = {}
            for path in hits.glob("*.json"):
                observed.update(json.loads(path.read_text()))
            for site in json.loads(inventory.read_text()):
                for outcome in site["outcomes"]:
                    self.assertGreater(observed.get(site["id"], {}).get(outcome, 0), 0,
                                       (site["expression"], outcome))

    def test_existing_unowned_output_is_preserved(self):
        with tempfile.TemporaryDirectory(prefix="bl-audit-output-test-") as temporary:
            out = Path(temporary)
            (out / "hits").mkdir()
            sentinel = out / "hits/user-file"
            sentinel.write_text("retain")
            result = subprocess.run(["python3", str(ROOT / "scripts/branch_coverage.py"),
                                     "--output", str(out)], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(sentinel.read_text(), "retain")


if __name__ == "__main__":
    unittest.main()
