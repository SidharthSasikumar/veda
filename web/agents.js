'use strict';
let crewSelection='', crewDashboardKey='', crewDetailKey='', crewConnected=true;

function minion(role, state='waiting', small=false) {
  const tools={
    compass:'<circle cx="84" cy="91" r="12" fill="#ecf1e4" stroke="#466b62" stroke-width="3"/><path d="m84 83 4 9-8 6z" fill="#e7785c"/>',
    map:'<path d="m70 83 10-4 10 4 10-4v24l-10 4-10-4-10 4z" fill="#e9eadb" stroke="#687fa4" stroke-width="2"/><path d="m80 81 0 21m10-18v21m-14-13 8-5 9 10" fill="none" stroke="#ab7cba" stroke-width="2"/>',
    terminal:'<rect x="69" y="83" width="31" height="24" rx="4" fill="#233e49" stroke="#87a5b9" stroke-width="2"/><path d="m75 89 5 5-5 5m9 0h9" fill="none" stroke="#b4e7a7" stroke-width="2"/>',
    bulb:'<path d="M83 79a11 11 0 0 0-6 20v5h12v-5a11 11 0 0 0-6-20" fill="#fff3ad" stroke="#b28c38" stroke-width="2"/><path d="M79 109h8m-4-36v-5m-15 14-5-3m35 2 5-3" stroke="#b28c38" stroke-width="2"/>',
    wrench:'<path d="m78 107-5-5 13-15a10 10 0 0 1 9-13l-4 7 5 3 5-6c3 10-5 15-11 12z" fill="#a6b1bc" stroke="#58697b" stroke-width="2"/>',
    book:'<path d="m70 82 14 3 14-3v24l-14 3-14-3z" fill="#fcf6db" stroke="#648369" stroke-width="2"/><path d="M84 85v24m-10-20 7 2m-7 4 7 2m7-6 6-2m-6 8 6-2" stroke="#95a184" stroke-width="2"/>'
  };
  return `<span class="minion ${small?'minion-small':''} is-${esc(state)}" aria-hidden="true"><svg viewBox="0 0 120 145" focusable="false"><ellipse class="minion-shadow" cx="57" cy="134" rx="29" ry="5" fill="#223d3320"/><g class="minion-body"><path d="m34 117-2 13h16l2-13m15 0 2 13h16l-3-13" fill="#334857"/><path class="minion-left-arm" d="M32 83q-20 5-14 22" fill="none" stroke="#edbd48" stroke-width="9" stroke-linecap="round"/><circle cx="19" cy="107" r="6" fill="#34434d"/><path d="M33 49c0-35 49-35 49 0v48c0 39-49 39-49 0z" fill="#f3cb56" stroke="#d9ac3e" stroke-width="1.5"/><path d="M33 84h49v20c0 28-49 28-49 0z" fill="${role.color}"/><path d="M34 79 42 89m37-10-9 10" stroke="${role.color}" stroke-width="6"/><rect x="43" y="85" width="27" height="22" rx="4" fill="${role.color}" stroke="#ffffff44"/><path d="M51 98h12" stroke="#ffffff88" stroke-width="2"/><path d="M29 52h57" stroke="#3b4a4e" stroke-width="7"/><g class="minion-eyes"><circle cx="45" cy="51" r="14" fill="#e9edf0" stroke="#87969d" stroke-width="5"/><circle cx="70" cy="51" r="14" fill="#e9edf0" stroke="#87969d" stroke-width="5"/><circle cx="47" cy="52" r="5" fill="#6c583d"/><circle cx="68" cy="52" r="5" fill="#6c583d"/><circle cx="48" cy="51" r="2.5" fill="#27312e"/><circle cx="67" cy="51" r="2.5" fill="#27312e"/><circle cx="49" cy="50" r="1" fill="white"/><circle cx="68" cy="50" r="1" fill="white"/></g><path d="M48 73q10 8 19-1" fill="none" stroke="#7c612d" stroke-width="2.5" stroke-linecap="round"/><path d="m49 20-5-9m13 7V8m6 11 5-8" stroke="#4d4b38" stroke-width="2.2" stroke-linecap="round"/><path class="minion-right-arm" d="M81 85q16 3 11 17" fill="none" stroke="#edbd48" stroke-width="9" stroke-linecap="round"/><g class="minion-tool">${tools[role.tool]}</g></g><g class="minion-spark" fill="#91b89a"><path d="m99 29 2 6 6 2-6 2-2 6-2-6-6-2 6-2z"/><circle cx="18" cy="58" r="2.5"/></g></svg></span>`;
}

function setAgentConnection(connected) {
  const changed=crewConnected!==connected;
  crewConnected=connected;
  document.body.classList.toggle('connection-stale',!connected);
  $('agent-connection-notice').hidden=connected;
  if(changed&&view==='workspace'&&tab==='agents')renderCrew(true);
}
function openAgentDashboard(){location.hash='agents';switchView('agents');}
function resetCrewSelection(){crewSelection='';crewDetailKey='';}
function renderAgentDashboard(force=false) {
  const query=$('crew-search').value.trim().toLowerCase(),filter=$('crew-filter').value;
  const all=library.investigations||[];
  const key=JSON.stringify([all,query,filter]);
  $('agents-live-count').textContent=all.filter(r=>r.status==='running').length;
  if(!force&&key===crewDashboardKey)return;
  crewDashboardKey=key;
  const rows=all.map(run=>({run,...VedaAgents.summary(run)}));
  $('crew-overview-stats').innerHTML=[[rows.length,'Investigations'],[rows.filter(x=>x.run.status==='running').length,'Working now'],[rows.filter(x=>x.issues||['failed','cancelled','interrupted'].includes(x.run.status)).length,'With issues or blockers'],[rows.filter(x=>x.run.reused_from).length,'Knowledge reuses']].map(([value,label])=>`<div><strong>${value}</strong><span>${label}</span></div>`).join('');
  const filtered=rows.filter(x=>(!query||`${x.run.repository} ${x.run.objective}`.toLowerCase().includes(query))&&(filter==='all'||filter==='running'&&x.run.status==='running'||filter==='attention'&&(x.issues||['failed','cancelled','interrupted'].includes(x.run.status))||filter==='completed'&&x.run.status==='completed'||filter==='reused'&&x.run.reused_from));
  $('crew-investigations').innerHTML=filtered.length?filtered.map(({run,agents,active,issues,finished,total})=>`<button class="investigation-crew-card ${run.status==='running'?'crew-running':''}" data-crew-run="${esc(run.id)}" aria-label="Open investigation: ${esc(run.repository||run.url)} · ${esc(run.objective)} · ${esc(time(run.created))}"><div class="crew-card-top"><span class="crew-repository">${esc(run.repository||run.url)}</span>${badge(run.reused_from?'reused':run.status)}</div><h3>${esc(run.objective)}</h3><div class="mini-crew">${agents.map(a=>`<div class="mini-agent">${minion(a,a.state,true)}<span>${a.name}</span><i class="crew-state-dot is-${a.state}" title="${a.label}"></i></div>`).join('')}</div><div class="crew-card-activity"><span class="crew-state-dot is-${active?'working':issues||['failed','cancelled','interrupted'].includes(run.status)?'issues':run.status==='running'?'waiting':'done'}"></span><span>${esc(active?`${active.name} · ${active.activity}`:run.reused_from?'Retrieved matching knowledge':issues?`${issues} recorded issues or blockers · open evidence`:run.status==='completed'?'Investigation finished · report saved':run.status==='running'?'Waiting for the next stage':`Investigation ${run.status}`)}</span></div><div class="crew-card-footer"><span>${finished}/${total} recorded steps finished${issues?` · ${issues} need review`:''}</span><span>Open crew <b aria-hidden="true">↗</b></span></div><div class="crew-card-time">${esc(time(run.created))} · ${esc(short(run.commit)||'Resolving revision')}</div></button>`).join(''):`<div class="crew-empty">${minion(VedaAgents.roles[0])}<h2>${all.length?'No matching investigations':'Your crew is ready.'}</h2><p>${all.length?'Try another search or filter.':'Start with a GitHub repository and an objective. Your characters will follow every stage.'}</p></div>`;
  $('crew-investigations').querySelectorAll('[data-crew-run]').forEach(button=>button.onclick=()=>selectRun(button.dataset.crewRun));
}
function renderCrew(force=false) {
  const focusedAgent=document.activeElement?.dataset?.agent;
  if(!current){$('crew-stations').innerHTML='<p class="muted">Loading this investigation’s crew…</p>';$('crew-agent-detail').replaceChildren();$('crew-feed').replaceChildren();$('crew-stage-banner').textContent='Loading saved activity…';return;}
  const data=VedaAgents.summary(current),agents=data.agents;
  if(!agents.some(a=>a.id===crewSelection))crewSelection=data.active?.id||agents.find(a=>['issues','blocked'].includes(a.state))?.id||'keeper';
  const key=JSON.stringify([current.id,current.status,current.stage,agents,current.events,crewSelection,crewConnected]);
  if(!force&&key===crewDetailKey)return;
  crewDetailKey=key;
  const selectedAgent=agents.find(a=>a.id===crewSelection);
  $('crew-live-label').textContent=!crewConnected?'Connection lost':current.status==='running'?'● Live activity':current.reused_from?'↺ Reused evidence':'Saved activity';
  $('crew-live-label').classList.toggle('is-live',current.status==='running'&&crewConnected);
  const note=!crewConnected?'Connection lost. Showing last saved activity; motion is paused.':current.reused_from?'This crew shows reused evidence. No new test or model work was performed.':current.status==='cancelled'||current.status==='interrupted'?'This investigation stopped. Completed evidence is retained; unfinished agents are no longer working.':'';
  $('crew-run-notice').hidden=!note;$('crew-run-notice').textContent=note;
  const active=data.active;
  const headline=active?`${active.name} is ${active.id==='thinker'?'reviewing the evidence':active.id==='tinkerer'?'investigating candidates': 'on the task'}`:current.status==='running'?'Your crew is getting ready':current.reused_from?'Knowledge retrieved':current.status==='completed'?'Investigation complete':`Investigation ${current.status}`;
  $('crew-stage-banner').innerHTML=`<div><span class="crew-state-dot is-${active?'working':data.issues||['failed','cancelled','interrupted'].includes(current.status)?'issues':current.status==='running'?'waiting':'done'}"></span><strong>${esc(headline)}</strong><p>${esc(active?.activity||(data.issues?`${data.issues} recorded issues or blockers. Select an agent to inspect the evidence.`:'Select an agent below to explore its recorded work.'))}</p></div><span>${data.finished}/${data.total}<small>recorded steps finished</small></span>`;
  $('crew-stations').innerHTML=agents.map((a,i)=>`<button class="agent-station is-${a.state} ${a.id===crewSelection?'selected':''}" style="--agent-color:${a.color};--agent-delay:${i*.17}s" data-agent="${a.id}" aria-pressed="${a.id===crewSelection}"><div class="agent-station-top"><span class="agent-number">0${i+1}</span><span class="agent-state is-${a.state}">${a.label}</span></div><div class="agent-scene"><div class="agent-desk"></div>${minion(a,a.state)}<span class="agent-task-symbol" aria-hidden="true">${a.state==='working'?'···':a.state==='done'?'✓':a.state==='reused'?'↺':['issues','blocked'].includes(a.state)?'!':'·'}</span></div><strong>${a.name}</strong><span class="agent-job">${a.job}</span><p>${esc(a.activity)}</p><span class="agent-task-count">${a.total?`${a.completed}/${a.total} recorded tasks finished`:a.id==='keeper'?'Report & shared memory':'No tasks recorded'}</span></button>`).join('');
  $('crew-stations').querySelectorAll('[data-agent]').forEach(button=>button.onclick=()=>{crewSelection=button.dataset.agent;renderCrew();});
  const a=selectedAgent;
  $('crew-agent-detail').innerHTML=`<div class="section-heading"><div><div class="eyebrow">SELECTED VIRTUAL AGENT</div><h2>${a.name} <span class="muted">/ ${a.job}</span></h2></div><span class="agent-state is-${a.state}">${a.label}</span></div><p class="muted">${a.description}</p><div class="agent-current-task"><span>${a.state==='working'?'CURRENT ACTIVITY':'RECORDED OUTCOME'}</span><p>${esc(a.activity)}</p></div><h3>Tasks & evidence</h3><div class="agent-task-list">${a.tasks.length?a.tasks.map(t=>`<button class="crew-task" data-task="${esc(t.id)}"><span class="crew-state-dot is-${t.state}"></span><span><strong>${esc(t.title)}</strong><small>${esc(t.id)} · ${esc(t.status)}${t.duration_seconds?` · ${Number(t.duration_seconds).toFixed(1)}s`:''}</small></span><span aria-hidden="true">↗</span></button>`).join(''):a.id==='keeper'&&(current.has_report??!!current.report)?'<button class="crew-task" id="crew-report-link"><span aria-hidden="true">▤</span><span><strong>Saved investigation report</strong><small>Open the report and shared evidence</small></span><span aria-hidden="true">↗</span></button>':`<p class="muted">${a.state==='waiting'?'Tasks will appear when this stage begins.':'No task records for this stage.'}</p>`}</div>`;
  $('crew-agent-detail').querySelectorAll('[data-task]').forEach(button=>button.onclick=()=>{selectedExperiment=button.dataset.task;showTab('experiments');renderExperiment();$('experiment-detail').scrollIntoView({block:'start'});});
  if($('crew-report-link'))$('crew-report-link').onclick=()=>showTab('report');
  if(focusedAgent){const restored=$('crew-stations').querySelector(`[data-agent="${focusedAgent}"]`);restored?.focus({preventScroll:true});}
  const events=current.events||[];$('crew-event-count').textContent=events.length+' EVENTS';
  $('crew-feed').innerHTML=events.length?[...events].reverse().slice(0,50).map(e=>`<div class="crew-feed-event"><time>${esc(time(e.at))}</time><p>${esc(e.message)}</p></div>`).join(''):'<p class="muted">Waiting for the first recorded activity…</p>';
}
function initAgents() {
  $('crew-welcome-art').innerHTML=minion(VedaAgents.roles[0],'done')+minion(VedaAgents.roles[1],'done')+minion(VedaAgents.roles[5],'done');
  $('agents-button').onclick=openAgentDashboard;$('back-to-agents').onclick=openAgentDashboard;
  $('crew-new').onclick=newInvestigation;
  $('crew-search').oninput=()=>renderAgentDashboard();$('crew-filter').onchange=()=>renderAgentDashboard();
  $('motion-button').onclick=()=>{const paused=document.documentElement.classList.toggle('motion-paused');$('motion-button').setAttribute('aria-pressed',String(paused));$('motion-button').textContent=paused?'Resume motion':'Pause motion';};
}
