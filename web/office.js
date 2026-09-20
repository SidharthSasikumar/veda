'use strict';
const officeState={runID:'',mode:'office',cursor:null,follow:true,playing:false,lastID:0,handoff:null,key:''};
const officeStations={scout:'Source desk',mapper:'Architecture board',tester:'Test station',thinker:'Review desk',tinkerer:'Experiment bench',keeper:'Knowledge library'};
let officePlayTimer=null,officeHandoffTimer=null,officeTimelineKey='';
const officeRole=id=>VedaAgents.roles.find(a=>a.id===id);
const officeName=id=>officeRole(id)?.name||'Workflow';
const officeTime=value=>value?new Date(value).toLocaleTimeString([],{hour:'2-digit',minute:'2-digit',second:'2-digit'}):'—';
function stopOfficePlayback(){clearTimeout(officePlayTimer);officePlayTimer=null;officeState.playing=false;}
function resetOffice(){stopOfficePlayback();clearTimeout(officeHandoffTimer);Object.assign(officeState,{runID:'',cursor:null,follow:true,lastID:0,handoff:null,key:''});officeTimelineKey='';}
function leaveOffice(){stopOfficePlayback();clearTimeout(officeHandoffTimer);officeState.handoff=null;officeState.key='';}
function officeSelectAgent(id){crewSelection=id;officeState.follow=false;renderOffice(true);}
function officeOpenTask(id){selectedExperiment=id;showTab('experiments');renderExperiment();$('experiment-detail').scrollIntoView({block:'start',behavior:'auto'});}
function officeSeek(index){
  if(!current)return;
  const event=VedaOffice.events(current)[index];if(!event)return;
  officeState.cursor=event.id;officeState.handoff=null;clearTimeout(officeHandoffTimer);
  if(officeState.follow)crewSelection=event.recipient_id||event.actor_id||crewSelection;
  renderOffice(true);
}
function scheduleOfficePlayback(){
  clearTimeout(officePlayTimer);
  if(!officeState.playing)return;
  officePlayTimer=setTimeout(()=>{
    if(!current||view!=='workspace'||tab!=='agents'||document.hidden){stopOfficePlayback();return;}
    const events=VedaOffice.events(current),index=events.findIndex(e=>e.id===officeState.cursor);
    if(index>=events.length-1){stopOfficePlayback();renderOffice(true);return;}
    officeSeek(index+1);scheduleOfficePlayback();
  },1200);
}
function initOffice(){
  $('office-stations').innerHTML=VedaAgents.roles.map((a,i)=>`<button class="office-station" data-agent="${a.id}" style="--station-x:${[17,50,83][i%3]}%;--station-y:${i<3?78:366}px" aria-pressed="false"><span class="office-station-art"><span class="office-desk" aria-hidden="true"></span><span class="office-monitor" aria-hidden="true"></span>${minion(a)}</span><strong>${a.name}</strong><small>${officeStations[a.id]}</small><span class="office-station-state"></span></button>`).join('');
  $('office-stations').querySelectorAll('[data-agent]').forEach(b=>b.onclick=()=>officeSelectAgent(b.dataset.agent));
  for(const mode of ['office','timeline'])$(mode+'-view').onclick=()=>{officeState.mode=mode;renderOffice(true);};
  $('office-follow').onchange=e=>{officeState.follow=e.target.checked;renderOffice(true);};
  $('office-cursor').oninput=e=>{stopOfficePlayback();officeSeek(Number(e.target.value));};
  $('office-latest').onclick=()=>{stopOfficePlayback();officeState.cursor=null;officeState.handoff=null;renderOffice(true);};
  $('office-play').onclick=()=>{
    if(officeState.playing){stopOfficePlayback();renderOffice(true);return;}
    if(!current)return;
    const events=VedaOffice.events(current);if(!events.length)return;
    if(officeState.cursor===null||officeState.cursor===events.at(-1).id)officeSeek(0);
    officeState.playing=true;renderOffice(true);scheduleOfficePlayback();
  };
  $('office-evidence-table').onclick=()=>{
    if(!current)return;
    const events=VedaOffice.events(current),frame=officeState.cursor!==null?VedaOffice.at(current,officeState.cursor):current;
    const handoff=VedaOffice.events(frame).findLast(e=>e.type==='artifact.handoff');
    if(handoff){stopOfficePlayback();officeSeek(events.indexOf(handoff));}
    else if(frame.reused_from)showTab('knowledge');
    else {const task=frame.experiments?.findLast(t=>VedaAgents.roleFor(t)==='tester')||frame.experiments?.at(-1);if(task)officeOpenTask(task.id);}
  };
  document.addEventListener('visibilitychange',()=>{document.body.classList.toggle('page-hidden',document.hidden);if(document.hidden)leaveOffice();else if(view==='workspace'&&tab==='agents')renderOffice(true);});
}
function updateOfficeHandoff(history){
  const latest=history.at(-1)?.id||0;
  if(current.id!==officeState.runID){officeState.runID=current.id;officeState.lastID=latest;return;}
  const fresh=VedaOffice.handoffsSince(current,officeState.lastID);
  officeState.lastID=latest;
  if(officeState.cursor!==null||!crewConnected||document.hidden||!fresh.length||['cancelled','interrupted','failed'].includes(current.status))return;
  // Coalesce fast transitions into the latest handoff; every event stays in history.
  officeState.handoff=fresh.at(-1);clearTimeout(officeHandoffTimer);
  officeHandoffTimer=setTimeout(()=>{officeState.handoff=null;if(view==='workspace'&&tab==='agents')renderOffice(true);},4500);
}
function renderOffice(force=false){
  if(!current){$('crew-stage-banner').textContent='Loading this investigation…';$('office-scene').hidden=true;$('crew-agent-detail').textContent='Loading saved activity…';return;}
  const history=VedaOffice.events(current),canReplay=VedaOffice.supportsReplay(current);
  updateOfficeHandoff(history);
  const replay=officeState.cursor!==null&&canReplay;
  const frame=replay?VedaOffice.at(current,officeState.cursor):current;
  const event=replay?history.find(e=>e.id===officeState.cursor):null;
  const data=VedaAgents.summary(frame),agents=data.agents;
  const handoff=event?.type==='artifact.handoff'?event:!replay&&crewConnected?officeState.handoff:null;
  if(officeState.follow)crewSelection=event?.recipient_id||data.active?.id||event?.actor_id||crewSelection;
  if(!agents.some(a=>a.id===crewSelection))crewSelection=data.active?.id||agents.find(a=>['issues','blocked'].includes(a.state))?.id||'keeper';
  const key=JSON.stringify([current.id,frame.status,frame.stage,agents,history.length,current.events?.length,crewSelection,crewConnected,officeState.mode,officeState.cursor,officeState.follow,officeState.playing,handoff?.id]);
  if(!force&&key===officeState.key)return;officeState.key=key;
  const focusedTask=document.activeElement?.dataset?.task;
  $('crew-live-label').textContent=replay?'Recorded replay':!crewConnected?'Connection lost':current.status==='running'?'● Live activity':current.reused_from?'Reused evidence':'Saved activity';
  $('crew-live-label').classList.toggle('is-live',!replay&&current.status==='running'&&crewConnected);
  let notice=!crewConnected?'Connection lost. Showing last saved activity; character motion is paused.':frame.reused_from?`Matching evidence from ${current.reused_from}, originally recorded ${time(current.evidence_created)}. No new checks or model review are shown as running.`:['cancelled','interrupted'].includes(frame.status)?'Investigation stopped. Completed evidence is retained.':'';
  if(replay)notice=`Recorded replay · ${officeTime(event?.at)}. Opening a task shows its saved evidence; playback never executes work.`;
  $('crew-run-notice').hidden=!notice;$('crew-run-notice').textContent=notice;
  const active=data.active,headline=handoff?`${officeName(handoff.actor_id)} → ${officeName(handoff.recipient_id)} · evidence handoff`:active?`${active.name} is at work`:frame.status==='running'?'Waiting for the next recorded step':frame.reused_from?'Knowledge retrieved':frame.status==='completed'?'Investigation complete':`Investigation ${frame.status}`;
  $('crew-stage-banner').innerHTML=`<div><span class="crew-state-dot is-${active?'working':data.issues?'issues':'done'}"></span><strong>${esc(headline)}</strong><p>${esc(handoff?handoff.message:active?.activity||event?.message||(data.issues?`${data.issues} recorded issues or blockers. Select a character to inspect the evidence.`:'Report and saved outcomes are ready to explore.'))}</p></div><span>${data.finished}/${data.total}<small>recorded steps finished</small></span>`;
  $('office-follow').checked=officeState.follow;
  $('office-follow').disabled=replay?false:current.status!=='running';
  $('office-history-label').textContent=canReplay?`${history.length} recorded events`:'Earlier investigation · summary history';
  $('office-replay').hidden=!canReplay;
  $('office-replay').classList.toggle('is-replaying',replay);
  $('office-cursor').max=Math.max(0,history.length-1);
  const index=replay?history.findIndex(e=>e.id===officeState.cursor):history.length-1;
  $('office-cursor').value=Math.max(0,index);
  $('office-cursor-label').textContent=`${index+1} / ${history.length}`;
  $('office-play').textContent=officeState.playing?'Pause replay':'Play recorded events';
  $('office-latest').disabled=!replay;
  $('office-replay-caption').textContent=replay?`${officeState.playing?'Playing':'Paused'} · one event per step · ${officeTime(event?.at)} · ${event?.message||''}`:'Select a recorded event to revisit its state. Playback advances one event every 1.2 seconds.';
  $('office-scene').hidden=officeState.mode!=='office';$('office-timeline').hidden=officeState.mode!=='timeline';
  $('office-view').setAttribute('aria-pressed',String(officeState.mode==='office'));$('timeline-view').setAttribute('aria-pressed',String(officeState.mode==='timeline'));
  $('office-scene').classList.toggle('office-static',!crewConnected||replay&&!officeState.playing||!replay&&current.status!=='running');
  for(const a of agents){
    const station=$('office-stations').querySelector(`[data-agent="${a.id}"]`);
    station.dataset.state=a.state;station.setAttribute('aria-pressed',String(a.id===crewSelection));station.setAttribute('aria-label',`${a.name}, ${officeStations[a.id]}, ${a.label}`);
    station.querySelector('.minion').className=`minion is-${a.state}`;
    station.querySelector('.office-station-state').innerHTML=`<span class="crew-state-dot is-${a.state}"></span>${esc(a.label)}`;
    station.classList.toggle('is-handing',Boolean(handoff&&[handoff.actor_id,handoff.recipient_id].includes(a.id)));
  }
  $('office-handoff').hidden=!handoff;
  const newestHandoff=VedaOffice.events(frame).findLast(e=>e.type==='artifact.handoff');
  $('office-evidence-table').disabled=!newestHandoff&&!frame.reused_from&&!frame.experiments?.length;
  $('office-table-caption').textContent=frame.reused_from?'Open original knowledge':newestHandoff?`${officeName(newestHandoff.actor_id)} → ${officeName(newestHandoff.recipient_id)}`:frame.experiments?.length?'Open recorded evidence':'Waiting for evidence';
  if(handoff){
    $('office-handoff-caption').textContent=`${replay?'Recorded':'Recent'} handoff · ${officeName(handoff.actor_id)} → ${officeName(handoff.recipient_id)}`;
    if($('office-visitors').dataset.event!==`${current.id}:${handoff.id}`){
      $('office-visitors').dataset.event=`${current.id}:${handoff.id}`;
      $('office-visitors').innerHTML=[handoff.actor_id,handoff.recipient_id].map((id,i)=>{const role=officeRole(id);return role?`<button class="office-visitor ${i?'receiver':''}" data-person="${id}" aria-label="Inspect ${role.name} ${i?'receiving':'handing over'} evidence">${minion(role,'done')}</button>`:'';}).join('');
      $('office-visitors').querySelectorAll('[data-person]').forEach(b=>b.onclick=()=>officeSelectAgent(b.dataset.person));
    }
  }else{$('office-visitors').dataset.event='';$('office-visitors').replaceChildren();}
  renderOfficeInspector(agents.find(a=>a.id===crewSelection),frame,replay,event,handoff);
  if(focusedTask)[...$('crew-agent-detail').querySelectorAll('[data-task]')].find(b=>b.dataset.task===focusedTask)?.focus({preventScroll:true});
  renderOfficeTimeline(frame,replay);
  const visibleEvents=replay?frame.events:current.events||[];
  $('crew-event-count').textContent=visibleEvents.length+' EVENTS';
  $('crew-feed').innerHTML=visibleEvents.length?[...visibleEvents].reverse().map(e=>`<div class="crew-feed-event ${event?.id===e.id?'is-selected':''}"><time>${esc(officeTime(e.at))}</time><div>${e.version===1?`<button data-event="${e.id}">${esc(e.message)}</button>`:`<p>${esc(e.message)}</p>`}${e.type==='artifact.handoff'?`<p class="muted">${e.artifact_ids?.length||0} recorded evidence items</p>`:''}</div></div>`).join(''):'<p class="muted">Waiting for the first recorded activity…</p>';
  $('crew-feed').querySelectorAll('[data-event]').forEach(b=>b.onclick=()=>{stopOfficePlayback();officeSeek(history.findIndex(e=>e.id===Number(b.dataset.event)));});
}
function renderOfficeInspector(a,frame,replay,event,handoff){
  const currentTask=a.tasks.find(t=>t.state==='working')||a.tasks.at(-1);
  const tool=!replay&&currentTask?.command?.length?currentTask.command.join(' '):a.id==='thinker'?library.model||'Local model':currentTask?.kind||a.job;
  const taskLink=t=>`<button class="crew-task" data-task="${esc(t.id)}"><span class="crew-state-dot is-${t.state}"></span><span><strong>${esc(t.title)}</strong><small>${esc(t.id)} · ${esc(t.status)}${t.duration_seconds?` · ${Number(t.duration_seconds).toFixed(2)}s`:''}</small></span><span aria-hidden="true">↗</span></button>`;
  $('crew-agent-detail').innerHTML=`<div class="eyebrow">${replay?'AGENT AT SELECTED EVENT':'SELECTED AGENT'}</div><div class="office-portrait office-static">${minion(a,a.state)}</div><div class="section-heading"><h2>${a.name}<span class="muted">${a.job} · ${officeStations[a.id]}</span></h2><span class="agent-state is-${a.state}">${a.label}</span></div><div class="agent-current-task"><span>${replay?'RECORDED ACTIVITY':a.state==='working'?'CURRENT ACTIVITY':'RECORDED OUTCOME'}</span><p>${esc(a.activity)}</p></div><div class="office-meta">${currentTask?.started?`<span>Started ${esc(officeTime(currentTask.started))}</span>`:''}${currentTask?.duration_seconds?`<span>Recorded duration ${Number(currentTask.duration_seconds).toFixed(2)} seconds</span>`:''}</div><div class="office-tool">${esc(tool)}</div>${handoff?`<div class="office-evidence"><h3>Evidence handoff</h3><p class="muted">${officeName(handoff.actor_id)} → ${officeName(handoff.recipient_id)} · ${esc(officeTime(handoff.at))}</p>${(handoff.artifact_ids||[]).map(id=>{const t=current.experiments.find(x=>x.id===id);return t?taskLink({...t,state:VedaAgents.taskState(t,current)}):'';}).join('')}</div>`:''}<h3>${replay?'Tasks recorded by this point':'Tasks & evidence'}</h3><div class="agent-task-list">${a.tasks.length?a.tasks.map(taskLink).join(''):`<p class="muted">${a.state==='waiting'?'Waiting for its next task.':a.state==='skipped'?'This role was not enabled.':a.id==='keeper'?'Report and shared evidence.':'No task records for this role.'}</p>`}</div><div class="office-artifact-links">${a.id==='mapper'?'<button class="quiet-button" data-view="architecture">Open architecture</button>':''}${a.id==='tinkerer'?'<button class="quiet-button" data-view="changes">Review changes</button><button class="quiet-button" data-view="experiments">Open experiments</button>':''}${a.id==='keeper'?'<button class="quiet-button" data-view="report">Open report</button><button class="quiet-button" data-view="knowledge">Shared knowledge</button>':''}</div>${replay?'<p class="muted" style="margin-top:12px">Evidence links open the saved result, including later outcomes.</p>':''}`;
  $('crew-agent-detail').querySelectorAll('[data-task]').forEach(b=>b.onclick=()=>officeOpenTask(b.dataset.task));
  $('crew-agent-detail').querySelectorAll('[data-view]').forEach(b=>b.onclick=()=>showTab(b.dataset.view));
}
function renderOfficeTimeline(frame,replay){
  const key=JSON.stringify([current.id,current.status,frame.experiments?.map(t=>[t.id,t.status,t.duration_seconds]),officeState.cursor]);if(key===officeTimelineKey)return;officeTimelineKey=key;
  const tasks=VedaOffice.timeline(frame),history=VedaOffice.events(frame),hasHistory=VedaOffice.supportsReplay(current);
  const legacy=(frame.experiments||[]).map(t=>({id:t.id,title:t.title,actor:VedaAgents.roleFor(t),status:t.status,start:t.started,end:t.started&&t.duration_seconds?new Date(Date.parse(t.started)+t.duration_seconds*1000).toISOString():null,recorded:!t.started}));
  const rows=hasHistory?tasks:current.reused_from?[]:legacy;
  const stamp=s=>{const t=Date.parse(s);return Number.isFinite(t)?t:null;};
  const nowAt=replay?stamp(history.at(-1)?.at):current.status==='running'?Date.now():stamp(current.finished);
  const bounds=rows.flatMap(t=>[stamp(t.start),stamp(t.end)]).filter(t=>t!==null);
  if(nowAt!==null&&bounds.length)bounds.push(nowAt);
  const min=bounds.length?Math.min(...bounds):0,max=bounds.length?Math.max(...bounds):1,span=Math.max(1,max-min);
  $('office-timeline-note').textContent=frame.reused_from?'These tasks come from saved knowledge; no fresh execution timeline is claimed.':hasHistory?'Recorded task intervals. Select a bar or a task below to open its evidence.':'Summary history from saved task times. Detailed event replay starts with investigations created after this update.';
  $('office-time-axis').innerHTML=bounds.length?`<span>${esc(new Date(min).toLocaleTimeString())}</span><span>${esc(new Date(max).toLocaleTimeString())}</span>`:'';
  $('office-lanes').innerHTML=VedaAgents.roles.map(a=>`<div class="office-lane"><button data-person="${a.id}">${a.name}</button><div class="office-track">${rows.filter(t=>t.actor===a.id).map(t=>{
    const start=stamp(t.start)??stamp(t.end);if(start===null)return '';
    const end=stamp(t.end)??nowAt??start,left=Math.min(99.4,Math.max(0,100*(start-min)/span)),width=Math.max(.6,Math.min(100-left,100*(end-start)/span));
    const state=VedaAgents.taskState(t,frame),label=`${t.title} · ${t.status}${t.recorded?' · outcome recorded; start unavailable':''}`;
    return `<button class="office-bar is-${state}" data-task="${esc(t.id)}" aria-label="${esc(label)}" title="${esc(label)}" style="--bar-left:${left}%;--bar-width:${width}%"><span class="office-bar-label">${width>15?esc(t.title):''}</span></button>`;
  }).join('')||'<span class="office-track-empty">No interval recorded</span>'}</div></div>`).join('');
  $('office-task-list').innerHTML=(frame.experiments||[]).map(t=>`<button class="crew-task" data-task="${esc(t.id)}"><span class="crew-state-dot is-${VedaAgents.taskState(t,frame)}"></span><span><strong>${esc(t.title)}</strong><small>${esc(t.status)} · ${Number(t.duration_seconds||0).toFixed(2)}s</small></span><span>↗</span></button>`).join('')||'<p class="muted">No tasks recorded at this point.</p>';
  $('office-timeline').querySelectorAll('[data-task]').forEach(b=>b.onclick=()=>officeOpenTask(b.dataset.task));
  $('office-timeline').querySelectorAll('[data-person]').forEach(b=>b.onclick=()=>officeSelectAgent(b.dataset.person));
}
