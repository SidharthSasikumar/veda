'use strict';
const changeView={run:'',id:'',file:'',mode:'split',visual:'pair',query:'',key:'',viewed:new Set()};
const changeLabels={proposed:'Needs review',rejected:'Rejected',failed:'Failed',supported:'Measured improvement',improved:'Measured improvement',accepted:'Tests passed',not_supported:'Target not met'};
function changeURL(c,suffix){return '/api/investigations/'+encodeURIComponent(current.id)+'/changes/'+encodeURIComponent(c.id)+'/'+suffix;}
function openChange(id){changeView.id=id;changeView.file='';changeView.key='';showTab('changes');}
function renderChanges(force=false){
 if(!current)return;
 if(changeView.run!==current.id){Object.assign(changeView,{run:current.id,id:'',file:'',query:'',key:''});}
 const changes=VedaChanges.list(current);
 if(!changes.some(c=>c.id===changeView.id))changeView.id=changes.find(c=>c.status==='proposed')?.id||changes[0]?.id||'';
 const c=changes.find(c=>c.id===changeView.id);
 const key=JSON.stringify([current.id,current.status,changes,changeView.id,changeView.file,changeView.mode,changeView.visual,changeView.query,[...changeView.viewed],!!library.active]);
 if(!force&&key===changeView.key)return;changeView.key=key;
 $('changes-generate').disabled=!!library.active||current.status==='running';
 $('changes-total').textContent=changes.length+' SUGGESTIONS';
 $('changes-empty').hidden=!!c;$('changes-review').hidden=!c;
 $('changes-empty-message').textContent=current.status==='running'?'Suggestions will appear here as this investigation progresses.':current.request.changes?'The model did not produce a reviewable change. Inspect the drafting task for its recorded outcome, or refine the objective and investigate again.':'This investigation has no saved patches. Generate suggestions to start a new investigation with the same objective and source ref.';
 if(!c)return;
 const select=$('change-select');select.innerHTML=changes.map(x=>`<option value="${esc(x.id)}" ${x.id===c.id?'selected':''}>${esc(x.id+' · '+x.title)}</option>`).join('');
 const files=VedaChanges.parse(c.patch),totalAdded=files.reduce((n,f)=>n+f.added,0),totalRemoved=files.reduce((n,f)=>n+f.removed,0);
 if(!files.some(f=>f.path===changeView.file))changeView.file=files[0]?.path||'';
 $('change-title').textContent=c.title;
 $('change-rationale').textContent=c.rationale||'Inspect the source change and its recorded validation before using it.';
 $('change-state').textContent=changeLabels[c.status]||c.status;$('change-state').className='change-status '+(c.status==='proposed'?'proposed':['rejected','failed'].includes(c.status)?'rejected':'recorded');
 $('change-validation').textContent=c.validation||'No validation result was recorded.';
 $('change-provenance').textContent=`Base ${short(current.repository.commit)} · ${c.legacy?'Paths are relative to the benchmark module':'Independent suggestion against the captured revision'}${current.reused_from?' · reused evidence':''}`;
 $('change-counts').innerHTML=`${files.length} files <span class="diff-added">+${totalAdded}</span> <span class="diff-removed">−${totalRemoved}</span>`;
 $('change-download').hidden=!c.patch;$('change-download').href=changeURL(c,'patch');
 $('change-evidence').hidden=!c.experiment_id;$('change-evidence').onclick=()=>officeOpenTask(c.experiment_id);
 $('change-filter').value=changeView.query;
 $('diff-split').setAttribute('aria-pressed',String(changeView.mode==='split'));$('diff-unified').setAttribute('aria-pressed',String(changeView.mode==='unified'));
 $('change-file-list').innerHTML=files.filter(f=>f.path.toLowerCase().includes(changeView.query.toLowerCase())).map(f=>`<button class="change-file ${f.path===changeView.file?'selected':''}" data-path="${esc(f.path)}" ${f.path===changeView.file?'aria-current="true"':''}><span>${changeView.viewed.has(current.id+':'+c.id+':'+f.path)?'✓':'◇'}</span><span>${esc(f.path)}</span><small><b class="diff-added">+${f.added}</b> <b class="diff-removed">−${f.removed}</b></small></button>`).join('')||'<p class="muted">No matching changed files.</p>';
 $('change-file-list').querySelectorAll('[data-path]').forEach(b=>b.onclick=()=>{changeView.file=b.dataset.path;renderChanges(true)});
 const file=files.find(f=>f.path===changeView.file);
 $('change-diff').innerHTML=file?diffFileHTML(file,c):'<div class="change-no-patch">No applicable patch was saved. The proposal was rejected before it could be applied to a private copy.</div>';
 const viewed=$('change-diff').querySelector('input[data-viewed]');if(viewed)viewed.onchange=()=>{const k=current.id+':'+c.id+':'+file.path;if(viewed.checked)changeView.viewed.add(k);else changeView.viewed.delete(k);renderChanges(true)};
 $('change-raw').textContent=c.patch||'No patch recorded.';
 renderChangePreview(c);
}
function diffFileHTML(file,c){
 const isSplit=changeView.mode==='split',mark=r=>r?.type==='added'?'+':r?.type==='removed'?'−':r?.type==='note'?'\\':' ';
 const cell=(r,side)=>`<td class="diff-number ${r?.type||'empty'}">${r?.[side]??''}</td><td class="diff-code ${r?.type||'empty'}"><span class="diff-sign" aria-hidden="true">${mark(r)}</span><code>${esc(r?.text||'')}</code></td>`;
 const hunks=file.hunks.map(h=>`<tr class="diff-hunk"><td colspan="${isSplit?4:3}">${esc(h.header)}</td></tr>${isSplit?VedaChanges.splitRows(h.rows).map(r=>`<tr>${cell(r.left,'old')}${cell(r.right,'new')}</tr>`).join(''):h.rows.map(r=>`<tr><td class="diff-number ${r.type}">${r.old??''}</td><td class="diff-number ${r.type}">${r.new??''}</td><td class="diff-code ${r.type}"><span class="diff-sign" aria-hidden="true">${mark(r)}</span><code>${esc(r.text)}</code></td></tr>`).join('')}`).join('');
 const viewed=changeView.viewed.has(current.id+':'+c.id+':'+file.path);
 return `<div class="diff-file-heading"><strong>${esc(file.path)}</strong><label title="Tracks files viewed during this page session"><input type="checkbox" data-viewed ${viewed?'checked':''}> Viewed</label></div>${file.oldPath&&file.newPath&&file.oldPath!==file.newPath?`<p class="muted diff-rename">Renamed from ${esc(file.oldPath)}</p>`:''}<div class="diff-scroll" tabindex="0" role="region" aria-label="Code diff for ${esc(file.path)}"><table class="code-diff ${isSplit?'split':'unified'}" aria-label="${isSplit?'Side by side':'Unified'} code changes"><thead>${isSplit?'<tr><th colspan="2">Original</th><th colspan="2">Suggested</th></tr>':'<tr><th>Old</th><th>New</th><th>Change</th></tr>'}</thead><tbody>${hunks||`<tr><td colspan="${isSplit?4:3}" class="diff-meta">${esc(file.meta.join('\n')||'No textual hunks in this file.')}</td></tr>`}</tbody></table></div>`;
}
function renderChangePreview(c){
 const p=c.preview,available=p&&['captured','partial'].includes(p.status)&&p.hashes?.before&&p.hashes?.after;
 $('change-preview').hidden=!c.ui_impact&&!p;
 $('preview-state').textContent=available?(p.status==='partial'?'Partial capture':'Captured comparison'):'Preview unavailable';
 $('preview-reason').textContent=p?.reason||'A UI effect is possible. Enable UI previews on a new investigation to capture a supported static HTML page.';
 $('preview-controls').hidden=!available;$('preview-images').hidden=!available;$('preview-details').hidden=!available;
 if(!available)return;
 $('preview-details').textContent=`${p.page} · ${p.width} × ${p.height} · ${time(p.captured)} · ${Number(p.changed_percent).toFixed(2)}% pixels changed (not a quality score)`;
 $('preview-warnings').textContent=(p.warnings||[]).join(' ');
 const before=changeURL(c,'preview/before'),after=changeURL(c,'preview/after'),difference=changeURL(c,'preview/difference');
 for(const mode of ['pair','swipe','difference'])$('preview-'+mode).setAttribute('aria-pressed',String(changeView.visual===mode));
 if(changeView.visual==='pair')$('preview-images').innerHTML=`<div class="preview-pair"><figure><figcaption>Before · captured revision</figcaption><a href="${before}" target="_blank" rel="noopener"><img src="${before}" alt="Before the suggested change" width="1280" height="900"></a></figure><figure><figcaption>After · ${esc(c.id)}</figcaption><a href="${after}" target="_blank" rel="noopener"><img src="${after}" alt="After this independent suggested change" width="1280" height="900"></a></figure></div>`;
 else if(changeView.visual==='difference')$('preview-images').innerHTML=`<figure class="preview-difference"><figcaption>Changed pixels in magenta</figcaption><a href="${difference}" target="_blank" rel="noopener"><img src="${difference}" alt="Pixel difference with changed areas highlighted in magenta" width="1280" height="900"></a></figure>`;
 else $('preview-images').innerHTML=`<div class="preview-swipe"><img src="${after}" alt="After the suggested change" width="1280" height="900"><img id="preview-overlay" src="${before}" alt="Original page overlay" width="1280" height="900" style="clip-path:inset(0 50% 0 0)"><span class="swipe-before">Before</span><span class="swipe-after">After</span></div><label class="preview-slider-label">Before / after boundary<input id="preview-slider" type="range" min="0" max="100" value="50" aria-label="Before and after screenshot comparison"></label>`;
 if($('preview-slider'))$('preview-slider').oninput=e=>$('preview-overlay').style.clipPath=`inset(0 ${100-Number(e.target.value)}% 0 0)`;
 $('preview-images').querySelectorAll('img').forEach(img=>img.onerror=()=>{if(!img.nextElementSibling?.classList.contains('preview-load-error')){const note=document.createElement('p');note.className='preview-load-error';note.textContent='This capture could not be loaded or failed its integrity check.';img.insertAdjacentElement('afterend',note);}});
}
function initChanges(){
 $('change-select').onchange=e=>{changeView.id=e.target.value;changeView.file='';renderChanges(true)};
 $('change-filter').oninput=e=>{changeView.query=e.target.value;const start=e.target.selectionStart;renderChanges(true);$('change-filter').focus();$('change-filter').setSelectionRange(start,start)};
 for(const mode of ['split','unified'])$('diff-'+mode).onclick=()=>{changeView.mode=mode;renderChanges(true)};
 for(const mode of ['pair','swipe','difference'])$('preview-'+mode).onclick=()=>{changeView.visual=mode;renderChanges(true)};
 $('changes-generate').onclick=()=>{if(current)submit({...current.request,ref:current.repository.commit||current.request.ref,model:true,changes:true,previews:true,force:true})};
}
