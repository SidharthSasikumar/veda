const {test}=require('node:test');
const assert=require('node:assert/strict');
const office=require('./office-model.js');
const agents=require('./agent-model.js');
const source={id:'EXP-1',kind:'source',title:'Capture source',status:'running',started:'2026-09-20T10:00:00Z'};
const base={id:'one',history_version:1,status:'completed',stage:'finished',request:{model:true},report:'Final report',experiments:[{...source,status:'passed',stdout:'FUTURE OUTPUT'}],events:[
 {version:1,id:1,type:'run.started',at:'2026-09-20T10:00:00Z'},
 {version:1,id:2,type:'stage.started',stage:'cloning',actor_id:'scout',at:'2026-09-20T10:00:00Z'},
 {version:1,id:3,type:'task.started',stage:'cloning',actor_id:'scout',task_id:'EXP-1',task:source,at:'2026-09-20T10:00:00Z'},
 {version:1,id:4,type:'task.completed',actor_id:'scout',task_id:'EXP-1',task:{...source,status:'passed',conclusion:'Captured source'},at:'2026-09-20T10:00:02Z'},
 {version:1,id:5,type:'artifact.handoff',actor_id:'scout',recipient_id:'mapper',artifact_ids:['EXP-1'],at:'2026-09-20T10:00:03Z'},
 {version:1,id:6,type:'run.finished',status:'completed',at:'2026-09-20T10:00:04Z'}
]};
test('replay contains only task outcomes known at the selected event',()=>{
 const before=office.at(base,3);assert.equal(before.experiments[0].status,'running');assert.equal(before.experiments[0].stdout,undefined);assert.equal(before.report,'');assert.equal(before.has_report,false);
 assert.equal(agents.crew(before).find(a=>a.id==='scout').state,'working');
 assert.equal(agents.crew(office.at(base,4)).find(a=>a.id==='scout').state,'done');
 assert.equal(office.at(base,6).status,'completed');assert.equal(office.at(base,6).has_report,true);
 assert.equal(base.experiments[0].stdout,'FUTURE OUTPUT');
});
test('legacy history is never fabricated and duplicate events cannot replay handoffs twice',()=>{
 assert.equal(office.at({...base,history_version:undefined},4),null);
 const duplicate={...base,events:[...base.events,base.events[4]].reverse()};
 assert.deepEqual(office.events(duplicate).map(e=>e.id),[1,2,3,4,5,6]);
 assert.equal(office.handoffsSince(duplicate,4).length,1);assert.equal(office.handoffsSince(duplicate,5).length,0);
});
test('reuse does not relabel previous evidence as newly executed tasks',()=>{
 const run={...base,reused_from:'original',evidence_created:'2026-09-19',events:[...base.events.slice(0,4),{version:1,id:5,type:'knowledge.reused',actor_id:'keeper'},{version:1,id:6,type:'run.finished',status:'completed'}]};
 assert.equal(office.at(run,4).reused_from,'');
 assert.equal(agents.crew(office.at(run,6)).find(a=>a.id==='scout').state,'reused');
 assert.deepEqual(office.timeline(run),[]);
});
test('task timeline uses recorded start/end and leaves retrospective candidates without invented starts',()=>{
 const run={...base,events:[...base.events,{version:1,id:7,type:'task.recorded',actor_id:'tinkerer',task_id:'OPT-1',task:{id:'OPT-1',title:'Candidate',kind:'candidate',status:'REJECTED'},at:'2026-09-20T10:00:04Z'}]};
 const rows=office.timeline(run);assert.equal(rows[0].start,'2026-09-20T10:00:00Z');assert.equal(rows[0].end,'2026-09-20T10:00:02Z');assert.equal(rows[1].start,null);assert.equal(rows[1].recorded,true);
});
test('cancelled task replay never leaves a worker processing',()=>{
 const run={...base,events:[...base.events.slice(0,3),{version:1,id:4,type:'task.cancelled',actor_id:'scout',task_id:'EXP-1',task:{...source,status:'cancelled'}},{version:1,id:5,type:'run.finished',status:'cancelled'}]};
 assert.equal(agents.crew(office.at(run,5)).some(a=>a.state==='working'),false);
});
