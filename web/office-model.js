/* Recorded events -> office state. Playback never invokes tools or changes a run. */
'use strict';
const VedaOffice = (() => {
  function events(run) {
    const seen=new Set();
    return (run.events||[]).filter(e=>e.version===1&&e.id>0).sort((a,b)=>a.id-b.id).filter(e=>{if(seen.has(e.id))return false;seen.add(e.id);return true;});
  }
  function supportsReplay(run) { return run.history_version===1&&events(run).length>0; }
  function at(run,id) {
    if(!supportsReplay(run))return null;
    const history=events(run).filter(e=>e.id<=id);
    const frame={...run,status:'running',stage:'queued',report:'',has_report:false,reused_from:'',evidence_created:'',experiments:[],events:history};
    const tasks=new Map();
    for(const e of history){
      if(e.type==='stage.started'||e.type==='activity')frame.stage=e.stage;
      if(e.task){tasks.set(e.task.id,{...e.task});frame.stage=e.type==='task.started'?e.stage:'between_steps';}
      if(e.type==='artifact.handoff')frame.stage='between_steps';
      if(e.type==='knowledge.reused'){
        frame.reused_from=run.reused_from;frame.evidence_created=run.evidence_created;frame.stage='reusing';
        tasks.clear();for(const task of run.experiments||[])tasks.set(task.id,{...task});
      }
      if(e.type==='run.finished'){frame.status=e.status;frame.stage='finished';frame.has_report=true;}
    }
    frame.experiments=[...tasks.values()];
    // A recorded outcome can be inspected without implying its role is still busy.
    return frame;
  }
  function handoffsSince(run,lastID) {return events(run).filter(e=>e.id>lastID&&e.type==='artifact.handoff');}
  function timeline(run) {
    if(run.reused_from)return [];
    const history=events(run),tasks=new Map();
    for(const e of history){
      if(!e.task)continue;
      const old=tasks.get(e.task_id)||{id:e.task_id,actor:e.actor_id,title:e.task.title,start:null,end:null,status:e.task.status,eventID:e.id};
      if(e.type==='task.started')old.start=e.at;
      else old.end=e.at;
      Object.assign(old,{status:e.task.status,eventID:e.id,recorded:e.type==='task.recorded'});
      tasks.set(e.task_id,old);
    }
    return [...tasks.values()];
  }
  return {events,supportsReplay,at,handoffsSince,timeline};
})();
if(typeof module!=='undefined'&&module.exports)module.exports=VedaOffice;
