const {test}=require('node:test');
const assert=require('node:assert/strict');
const changes=require('./changes-model.js');
test('unified patches preserve file names, line numbers, plus-like source, and final newline markers',()=>{
 const files=changes.parse('diff --git a/web/app.js b/web/app.js\n--- a/web/app.js\n+++ b/web/app.js\n@@ -8,2 +8,3 @@ function value()\n old();\n-remove();\n++++counter;\n+<script>alert(1)</script>\n\\ No newline at end of file\n');
 assert.equal(files[0].path,'web/app.js');assert.equal(files[0].added,2);assert.equal(files[0].removed,1);
 assert.deepEqual(files[0].hunks[0].rows.slice(0,4).map(r=>[r.old,r.new,r.text]),[[8,8,'old();'],[9,null,'remove();'],[null,9,'+++counter;'],[null,10,'<script>alert(1)</script>']]);
 assert.equal(files[0].hunks[0].rows.at(-1).type,'note');
});
test('added, deleted, multiple and quoted Unicode paths remain separate',()=>{
 const files=changes.parse('diff --git a/new.css b/new.css\n--- /dev/null\n+++ b/new.css\n@@ -0,0 +1 @@\n+body{}\ndiff --git a/old.css b/old.css\n--- a/old.css\n+++ /dev/null\n@@ -1 +0,0 @@\n-body{}\ndiff --git "a/\\303\\251 space.css" "b/\\303\\251 space.css"\n--- "a/\\303\\251 space.css"\n+++ "b/\\303\\251 space.css"\n@@ -1 +1 @@\n-a\n+b\n');
 assert.deepEqual(files.map(f=>f.path),['new.css','old.css','é space.css']);assert.equal(files[0].oldPath,'');assert.equal(files[1].newPath,'');
});
test('split rows align replacements without duplicating additions or losing context',()=>{
 const rows=[{type:'context',text:'same'},{type:'removed',text:'one'},{type:'removed',text:'two'},{type:'added',text:'new'},{type:'context',text:'end'}];
 const split=changes.splitRows(rows);assert.equal(split.length,4);assert.equal(split[1].left.text,'one');assert.equal(split[1].right.text,'new');assert.equal(split[2].left.text,'two');assert.equal(split[2].right,null);
});
test('legacy rejected candidate patches are shown without relabeling them as approved',()=>{
 const rows=changes.list({changes:[{id:'CHG-001',status:'proposed'}],experiments:[{id:'OPT-1',status:'REJECTED',patch:'patch',title:'Candidate'},{id:'EXP-1',status:'passed'}]});
 assert.equal(rows.length,2);assert.equal(rows[1].status,'rejected');assert.equal(rows[1].legacy,true);
});
