const {test}=require('node:test');
const assert=require('node:assert/strict');
const model=require('./agent-model.js');
const source={id:'one',kind:'source',title:'Clone and capture revision',status:'passed'};
const check={id:'two',kind:'runtime',title:'Go tests',status:'running'};
const run={status:'running',stage:'checks',request:{model:true,optimize:false},experiments:[source,check]};

test('live roles derive from actual tasks and only the current stage works',()=>{
  const crew=model.crew(run);
  assert.deepEqual(crew.filter(a=>a.state==='working').map(a=>a.id),['tester']);
  assert.equal(crew.find(a=>a.id==='scout').state,'done');
  assert.equal(crew.find(a=>a.id==='thinker').state,'waiting');
  assert.equal(crew.find(a=>a.id==='tinkerer').state,'skipped');
  assert.equal(model.crew({...run,stage:'queued',experiments:[]}).some(a=>a.state==='working'),false);
});
test('cancelled or interrupted work is never shown as still processing',()=>{
  for(const status of ['cancelled','interrupted','failed']){
    const crew=model.crew({...run,status,stage:'finished',report:'Saved partial report'});
    assert.equal(crew.some(a=>a.state==='working'),false);
    assert.equal(crew.find(a=>a.id==='tester').state,status==='cancelled'?'cancelled':'interrupted');
    assert.equal(crew.find(a=>a.id==='keeper').state,'done');
  }
});
test('completion preserves failed and blocked checks instead of celebrating every role',()=>{
  const summary=model.summary({...run,status:'completed',stage:'finished',report:'Report',experiments:[source,{...check,status:'failed'},{kind:'model',status:'blocked',title:'Local review'}]});
  assert.equal(summary.issues,2);
  assert.equal(summary.agents.find(a=>a.id==='tester').state,'issues');
  assert.equal(summary.agents.find(a=>a.id==='thinker').state,'blocked');
  assert.equal(summary.agents.find(a=>a.id==='keeper').state,'done');
});
test('cached evidence does not animate as fresh execution',()=>{
  const cached={...run,status:'completed',stage:'finished',reused_from:'old',report:'Report',experiments:[source,{...check,status:'passed'}]};
  const summary=model.summary(cached);
  assert.equal(summary.active,null);
  assert.equal(summary.agents.find(a=>a.id==='tester').state,'reused');
  assert.equal(summary.agents.find(a=>a.id==='keeper').state,'reused');
  assert.equal(cached.experiments[0].status,'passed');
});
test('library summaries and full investigation detail produce consistent roles',()=>{
  const {experiments,...rest}=run;
  assert.deepEqual(model.crew({...rest,work:experiments,has_report:false}),model.crew(run));
});
test('candidate rejections remain visible and disabled work is never counted as performed',()=>{
  const crew=model.crew({status:'completed',stage:'finished',request:{model:false,optimize:true},report:'Report',experiments:[{kind:'candidate',title:'Baseline',status:'passed'},{kind:'candidate',title:'Proposal',status:'REJECTED'}]});
  assert.equal(crew.find(a=>a.id==='tinkerer').state,'issues');
  assert.equal(crew.find(a=>a.id==='thinker').state,'skipped');
  assert.equal(crew.find(a=>a.id==='scout').state,'not_started');
});
