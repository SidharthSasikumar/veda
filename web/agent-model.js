/* Pure presentation model: virtual roles reflect recorded work, never simulated workers. */
'use strict';
const VedaAgents = (() => {
  const roles = [
    {id:'scout', name:'Scout', job:'Repository explorer', description:'Clones the repository and captures its exact revision.', tool:'compass', color:'#397c7b'},
    {id:'mapper', name:'Mapper', job:'System architect', description:'Connects infrastructure and application components to source evidence.', tool:'map', color:'#8875b7'},
    {id:'tester', name:'Tester', job:'Evidence investigator', description:'Prepares runtimes and dependencies, then runs supported source and isolated checks.', tool:'terminal', color:'#5376a8'},
    {id:'thinker', name:'Thinker', job:'Local model reviewer', description:'Reviews the objective and observed evidence with the local model.', tool:'bulb', color:'#ba8844'},
    {id:'tinkerer', name:'Tinkerer', job:'Change researcher', description:'Drafts reviewable source changes, captures supported UI previews, and compares Go benchmark candidates.', tool:'wrench', color:'#b36d6e'},
    {id:'keeper', name:'Keeper', job:'Knowledge librarian', description:'Saves the report and preserves evidence for future investigations.', tool:'book', color:'#678552'}
  ];
  const stageRole = {cloning:'scout',environment:'tester',architecture:'mapper',checks:'tester',reasoning:'thinker',drafting:'tinkerer',optimizing:'tinkerer',reporting:'keeper',reusing:'keeper'};
  const labels = {working:'Working',waiting:'Waiting',done:'Done',issues:'Issues found',blocked:'Blocked',skipped:'Not enabled',not_started:'Not reached',cancelled:'Cancelled',interrupted:'Stopped',reused:'Reused evidence'};
  function roleFor(task) {
    if(task.kind==='source') return 'scout';
    if(task.kind==='model') return 'thinker';
    if(['candidate','change','preview'].includes(task.kind)) return 'tinkerer';
    if(task.kind==='static' && task.title==='Map declared architecture') return 'mapper';
    return 'tester';
  }
  function taskState(task,run) {
    const status=String(task.status||'').toLowerCase();
    if(status==='running') return run.status==='running'?'working':run.status==='cancelled'?'cancelled':'interrupted';
    if(['failed','rejected','not_supported'].includes(status)) return 'issues';
    if(status==='blocked') return 'blocked';
    if(status==='cancelled'||status==='interrupted') return status;
    if(status==='skipped') return 'skipped';
    if(['passed','supported','improved','accepted'].includes(status)) return run.reused_from?'reused':'done';
    return 'not_started';
  }
  function crew(run) {
    const tasks=Array.isArray(run.experiments)?run.experiments:(run.work||[]);
    const request=run.request||{};
    const latest=run.events?.at(-1)||run.activity;
    const live=run.status==='running';
    const focus=live?stageRole[run.stage]:null;
    return roles.map(role=>{
      const assigned=tasks.filter(t=>roleFor(t)===role.id).map(t=>({...t,state:taskState(t,run)}));
      let state='waiting', activity='Waiting for its turn.', completed=assigned.filter(t=>!['working','waiting','not_started'].includes(t.state)).length;
      if(role.id==='keeper') {
        const saved=run.has_report??Boolean(run.report);
        if(!live && saved) {state=run.reused_from?'reused':'done';activity=run.reused_from?'Matching evidence retrieved; its original timestamps are preserved.':'Report and evidence saved in the shared library.';}
        else if(focus==='keeper') {state='working';activity=latest?.message||'Saving the investigation.';}
        else if(!live) {state='not_started';activity='No saved report is available.';}
      } else if(focus===role.id) {
        state='working';activity=assigned.find(t=>t.state==='working')?.title||latest?.message||role.description;
        if(role.id==='tinkerer'&&latest?.message) activity=latest.message;
      } else if(assigned.some(t=>t.state==='working')) {
        state='working';activity=assigned.find(t=>t.state==='working').title;
      } else if(assigned.length) {
        state=['issues','blocked','cancelled','interrupted'].find(s=>assigned.some(t=>t.state===s)) || (assigned.every(t=>t.state==='skipped')?'skipped':run.reused_from?'reused':'done');
        const last=assigned.findLast(t=>t.state===state)||assigned.at(-1);
        activity=last.conclusion||`${last.title} · ${labels[last.state]}`;
      } else if((role.id==='thinker'&&!request.model)||(role.id==='tinkerer'&&!request.optimize&&!request.changes)) {
        state='skipped';activity=role.id==='thinker'?'Local model review was not enabled.':'Suggested changes and benchmark improvements were not enabled.';
      } else if(!live) {
        state='not_started';activity=run.status==='cancelled'?'Investigation stopped before this stage.':'No work was recorded for this stage.';
      }
      return {...role,state,label:labels[state],activity,tasks:assigned,completed,total:assigned.length};
    });
  }
  function summary(run) {
    const agents=crew(run),tasks=Array.isArray(run.experiments)?run.experiments:(run.work||[]);
    const issues=tasks.filter(t=>['issues','blocked'].includes(taskState(t,run))).length;
    const finished=tasks.filter(t=>!['working','waiting','not_started'].includes(taskState(t,run))).length;
    return {agents,issues,finished,total:tasks.length,active:agents.find(a=>a.state==='working')||null};
  }
  return {roles,crew,summary,roleFor,taskState,labels};
})();
if(typeof module!=='undefined'&&module.exports) module.exports=VedaAgents;
