'use strict';
const VedaChanges=(()=>{
 function gitPath(value){
  value=value.split('\t')[0];
  if(value.startsWith('"')&&value.endsWith('"')){
   const bytes=[];
   for(let i=1;i<value.length-1;i++){
    if(value[i]==='\\'){
     const oct=value.slice(i+1).match(/^[0-7]{3}/);if(oct){bytes.push(parseInt(oct[0],8));i+=3;continue;}
     const c=value[++i];bytes.push(({t:9,n:10,r:13,b:8,f:12,v:11})[c]??c.charCodeAt(0));
    }else bytes.push(...new TextEncoder().encode(value[i]));
   }
   value=new TextDecoder().decode(new Uint8Array(bytes));
  }
  return value==='/dev/null'?'':value.replace(/^[ab]\//,'');
 }
 function parse(patch){
  const files=[];let file=null,hunk=null,old=0,newLine=0,remainingOld=0,remainingNew=0;
  for(const line of String(patch||'').split('\n')){
   if(line.startsWith('diff --git ')){file={path:'',oldPath:'',newPath:'',added:0,removed:0,hunks:[],meta:[]};files.push(file);hunk=null;remainingOld=remainingNew=0;continue;}
   if(!file){if(!line.startsWith('--- '))continue;file={path:'',oldPath:'',newPath:'',added:0,removed:0,hunks:[],meta:[]};files.push(file);}
   const header=line.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$/);
   if(header){old=Number(header[1]);newLine=Number(header[3]);remainingOld=header[2]===undefined?1:Number(header[2]);remainingNew=header[4]===undefined?1:Number(header[4]);hunk={header:line,rows:[]};file.hunks.push(hunk);continue;}
   if(hunk&&(remainingOld>0||remainingNew>0)){
    const kind=line[0],text=line.slice(1);
    if(kind===' '){hunk.rows.push({type:'context',text,old:old++,new:newLine++});remainingOld--;remainingNew--;}
    else if(kind==='-'){hunk.rows.push({type:'removed',text,old:old++,new:null});remainingOld--;file.removed++;}
    else if(kind==='+'){hunk.rows.push({type:'added',text,old:null,new:newLine++});remainingNew--;file.added++;}
    else if(kind==='\\')hunk.rows.push({type:'note',text:line,old:null,new:null});
    continue;
   }
   if(line.startsWith('--- ')){file.oldPath=gitPath(line.slice(4));file.path=file.oldPath;}
   else if(line.startsWith('+++ ')){file.newPath=gitPath(line.slice(4));file.path=file.newPath||file.oldPath;}
   else if(line.startsWith('rename from ')){file.oldPath=gitPath(line.slice(12));file.path=file.oldPath;}
   else if(line.startsWith('rename to ')){file.newPath=gitPath(line.slice(10));file.path=file.newPath;}
   else if(line.startsWith('\\')&&hunk)hunk.rows.push({type:'note',text:line,old:null,new:null});
   else if(line)file.meta.push(line);
  }
  return files.map((f,i)=>({...f,path:f.path||'Patch file '+(i+1)}));
 }
 function splitRows(rows){
  const out=[];
  for(let i=0;i<rows.length;){
   if(rows[i].type==='context'||rows[i].type==='note'){out.push({left:rows[i],right:rows[i]});i++;continue;}
   const removed=[],added=[];
   while(i<rows.length&&!['context','note'].includes(rows[i].type)){(rows[i].type==='removed'?removed:added).push(rows[i++]);}
   for(let j=0;j<Math.max(removed.length,added.length);j++)out.push({left:removed[j]||null,right:added[j]||null});
  }
  return out;
 }
 function list(run){return [...(run.changes||[]),...(run.experiments||[]).filter(x=>x.patch).map(x=>({id:x.id,title:x.title,rationale:x.input,status:String(x.status).toLowerCase(),validation:x.conclusion,patch:x.patch,experiment_id:x.id,metrics:x.metrics,legacy:true}))];}
 return {parse,splitRows,list};
})();
if(typeof module!=='undefined'&&module.exports)module.exports=VedaChanges;
