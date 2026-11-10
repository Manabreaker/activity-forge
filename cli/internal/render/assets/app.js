(() => {
  'use strict';
  const initial = window.BOOTSTRAP;
  let state = initial, config = structuredClone(initial.plan.config), mode = 'preview', revision = initial.revision || 0, request = 0, timer, selectedDate = null;
  const $ = id => document.getElementById(id);
  const format = n => Number(n).toLocaleString('en-US');
  const clone = v => structuredClone(v);
  const dateOf = s => new Date(s + 'T12:00:00Z');
  const prettyDate = s => dateOf(s).toLocaleDateString('en-US', {month:'long', day:'numeric', year:'numeric', timeZone:'UTC'});
  const fields = {'start':'start','end':'end','preset':'preset','intensity':'intensity','randomness':'randomness','mean':'mean','min':'min','max':'max','inactive':'inactive_percent','seed':'seed','timezone':'timezone'};
  const effectFields = {'bursts':'work_bursts','rest':'rest_periods','streaks':'streaks','monthly':'monthly_variation','vacations':'vacation_count','vacation-min':'vacation_min','vacation-max':'vacation_max','trend':'trend'};
  const clockFields = {'work-start':'start','work-end':'end','spread':'spread_minutes','evening':'evening_probability','night':'night_probability'};
  function put(id, value) { const el=$(id); if(el.type==='checkbox') el.checked=!!value; else el.value=value; }
  function read(id) { const el=$(id); return el.type==='checkbox' ? el.checked : ['number','range'].includes(el.type) ? Number(el.value) : el.value; }
  function syncForm() {
    Object.entries(fields).forEach(([id,key])=>put(id,config[key]));
    Object.entries(effectFields).forEach(([id,key])=>put(id,config.effects[key] || (id==='trend'?'stable':0)));
    Object.entries(clockFields).forEach(([id,key])=>put(id,config.clock[key]*(['evening','night'].includes(id)?100:1) || config.clock[key]));
    put('work-start',config.clock.start); put('work-end',config.clock.end);
    put('weekends',config.weekends.mode);
    ['sat','sun'].forEach((s,i)=>{const d=config.weekends[i?'sunday':'saturday']; put(s+'-prob',d.probability*100);put(s+'-mult',d.multiplier);put(s+'-max',d.max);});
    put('author-name',config.author.name);put('author-email',config.author.email);
    updateOutputs();
  }
  function updateOutputs() { $('intensity-value').textContent=$('intensity').value+' / 10'; $('randomness-value').textContent=$('randomness').value+'%'; $('custom-weekends').hidden=$('weekends').value!=='custom'; }
  function collect() {
    const c=clone(config);
    Object.entries(fields).forEach(([id,key])=>c[key]=read(id));
    Object.entries(effectFields).forEach(([id,key])=>c.effects[key]=read(id));
    Object.entries(clockFields).forEach(([id,key])=>c.clock[key]=['evening','night'].includes(id)?read(id)/100:read(id));
    c.weekends.mode=read('weekends');
    ['sat','sun'].forEach((s,i)=>{c.weekends[i?'sunday':'saturday']={probability:read(s+'-prob')/100,multiplier:read(s+'-mult'),max:read(s+'-max')};});
    c.author={name:read('author-name'),email:read('author-email')};
    return c;
  }
  async function api(path, data) {
    const res=await fetch('/api/'+path,{method:data===undefined?'GET':'POST',headers:{'Content-Type':'application/json','X-ActivityForge-Token':initial.token},body:data===undefined?undefined:JSON.stringify(data)});
    if(!res.ok) { let msg; try { msg=(await res.json()).error; } catch { msg='Request failed ('+res.status+')'; } throw new Error(msg); }
    return res.json();
  }
  function showError(e) { $('error').textContent=e.message||String(e);$('error').hidden=false; }
  async function recalculate(preset) {
    if(initial.offline || state.frozen || state.kind!=='preview') return;
    const current=++request; $('status-badge').textContent='Recalculating…';
    try { const next=await api('preview',{config:collect(),revision:++revision,apply_preset:!!preset});if(current!==request)return;state=next;config=clone(next.plan.config);$('error').hidden=true;syncForm();render(); }
    catch(e) { if(current===request){showError(e);$('status-badge').textContent='Invalid settings';} }
  }
  function levels(counts) {
    const a=counts.filter(n=>n>0).sort((a,b)=>a-b);
    if(!a.length)return ()=>0;
    const q=[a[Math.floor((a.length-1)/4)],a[Math.floor((a.length-1)/2)],a[Math.floor(3*(a.length-1)/4)]];
    return n=>n<=0?0:1+q.filter(v=>n>v).length;
  }
  const currentLevel = {'NONE':0,'FIRST_QUARTILE':1,'SECOND_QUARTILE':2,'THIRD_QUARTILE':3,'FOURTH_QUARTILE':4};
  function count(d) {return mode==='current'?d.existing:mode==='diff'?d.new:d.projected;}
  function renderCalendar() {
    const p=state.plan, root=$('calendar'), days=p.days, offset=dateOf(days[0].date).getUTCDay(), weeks=Math.ceil((days.length+offset)/7);
    root.replaceChildren(); root.style.width=(44+weeks*14)+'px';
    ['Mon','Wed','Fri'].forEach((label,i)=>{const el=document.createElement('span');el.className='day-label';el.style.left='0';el.style.top=(24+(i*2+1)*14)+'px';el.textContent=label;root.append(el);});
    const getLevel=levels(days.map(count)); let prevMonth='';
    days.forEach((d,i)=>{
      const dt=dateOf(d.date),col=Math.floor((i+offset)/7),row=(i+offset)%7,month=d.date.slice(0,7);
      if(month!==prevMonth){const label=document.createElement('span');label.className='month-label';label.style.left=(43+col*14)+'px';label.style.top='0';label.textContent=dt.toLocaleDateString('en-US',{month:'short',timeZone:'UTC'});root.append(label);prevMonth=month;}
      const el=document.createElement('button');el.className='cell';el.dataset.date=d.date;el.setAttribute('role','gridcell');el.setAttribute('aria-label',prettyDate(d.date)+': '+count(d)+' contributions');el.dataset.level=getLevel(count(d));el.style.left=(43+col*14)+'px';el.style.top=(25+row*14)+'px';
      const existing=p.existing.days?.[d.date];
      if(mode==='current' && p.existing.known && existing){el.dataset.level=currentLevel[existing.contributionLevel]??getLevel(d.existing); if(/^#[a-f\d]{6}$/i.test(existing.color||'')) el.style.background=existing.color;}
      const tip=e=>{const t=$('tooltip');t.textContent=prettyDate(d.date)+'\nExisting contributions: '+(p.existing.known?format(d.existing):'unknown')+'\nNew commits: '+format(d.new)+'\nProjected total: '+format(d.projected);t.hidden=false;const b=el.getBoundingClientRect();t.style.left=Math.max(8,Math.min((e.clientX||b.left)+12,innerWidth-270))+'px';t.style.top=Math.max(8,Math.min((e.clientY||b.top)+16,innerHeight-130))+'px';};
      el.addEventListener('mouseenter',tip);el.addEventListener('focus',tip);el.addEventListener('mouseleave',()=>$('tooltip').hidden=true);el.addEventListener('blur',()=>$('tooltip').hidden=true);el.addEventListener('click',()=>dayDetails(d));root.append(el);
    });
    const total=days.reduce((sum,d)=>sum+count(d),0);
    $('calendar-title').textContent=(mode==='current'&&!p.existing.known?'Unknown current activity':format(total)+' '+(mode==='diff'?'new commits':'contributions'));
    $('calendar-caption').textContent=mode==='current'?'Current GitHub calendar · API color levels':mode==='diff'?'Only the additional commits in this plan':'Projected calendar · existing + planned commits';
  }
  function dayDetails(d) {
	selectedDate=d.date;
    $('day-title').textContent=prettyDate(d.date);$('day-description').textContent=(state.plan.existing.known?d.existing:'Unknown')+' existing · '+d.new+' new · '+d.projected+' projected';$('day-times').replaceChildren();
    (d.timestamps||[]).forEach(t=>{const el=document.createElement('code');el.textContent=t; $('day-times').append(el);});
    if(!d.timestamps?.length) $('day-times').textContent='No new commits scheduled.';
  }
  function renderStats() {
    const s=state.plan.statistics; $('statistics').replaceChildren();
    [['Active days',format(s.active_days),'days'],['Longest streak',format(s.longest_streak),'days'],['Daily average',s.average_active.toFixed(1),'per active day'],['Inactive days',format(s.inactive_days),'days'],['Weekday activity',s.weekday_percent.toFixed(0),'%'],['Weekend activity',s.weekend_percent.toFixed(0),'%'],['Maximum daily',format(s.max_daily),'contributions']].forEach(([label,n,unit])=>{const div=document.createElement('div');div.className='stat';const span=document.createElement('span');span.textContent=label;const b=document.createElement('b');b.textContent=n;const small=document.createElement('small');small.textContent=unit;b.append(small);div.append(span,b);$('statistics').append(div);});
    const chart=$('monthly-chart');chart.replaceChildren();const max=Math.max(1,...s.monthly.map(m=>m.total));s.monthly.forEach(m=>{const bar=document.createElement('div');bar.className='month-bar';bar.style.height=Math.max(3,105*m.total/max)+'px';bar.title=m.month+': '+m.total+' projected ('+m.existing+' existing, '+m.planned+' new)';const label=document.createElement('span');label.textContent=dateOf(m.month+'-01').toLocaleDateString('en-US',{month:'short',timeZone:'UTC'});bar.append(label);chart.append(bar);});
  }
  function render() {
    const p=state.plan,s=p.statistics,readOnly=initial.offline||state.frozen||state.kind!=='preview';
    document.body.classList.toggle('readonly',readOnly);
    $('settings').querySelectorAll('input,select,button').forEach(el=>el.disabled=readOnly);$('reset').disabled=readOnly;$('fetch-control').disabled=readOnly||!state.can_fetch;
    $('existing-total').textContent=p.existing.known?format(s.existing):'—';$('existing-note').textContent=p.existing.known?'@'+(p.existing.login||'GitHub')+' · cached calendar':'Not loaded · baseline unknown';$('planned-total').textContent='+'+format(s.planned);$('projected-total').textContent=format(s.total);$('projected-note').textContent=p.existing.known?'Existing + planned activity':'Planned activity · baseline unknown';
    $('period').textContent=p.config.start+' — '+p.config.end;$('unknown').hidden=p.existing.known;$('fetch').hidden=initial.offline||readOnly||!state.can_fetch;
    $('status-badge').textContent=initial.offline?'Offline snapshot':state.published?'Published':state.kind==='publish'?'Ready for approval':state.kind==='inspect'?'Verified local history':state.frozen?'Immutable plan':'Draft plan';
    $('eyebrow').textContent=state.kind==='inspect'?'LOCAL GIT INSPECTION':state.kind==='publish'?'FINAL PREVIEW · APPROVAL REQUIRED':'VISUAL PREVIEW';
    $('plan-id').textContent=p.id.slice(0,16)+' · seed '+p.config.seed;$('plan-label').textContent=state.frozen?'Immutable saved plan':'Deterministic plan';
    $('freeze').hidden=initial.offline||state.frozen||state.kind!=='preview';$('draft').hidden=initial.offline||!state.frozen||state.kind!=='preview';$('push').hidden=state.kind!=='publish'||initial.offline||state.published;$('push').disabled=!state.can_push;$('cancel').hidden=state.kind!=='publish'||initial.offline||state.published;
    $('back').hidden=state.kind!=='publish'||initial.offline||state.published;
    $('notice').textContent=state.notice||(state.saved_path?'Plan saved: '+state.saved_path:'');$('notice').hidden=!$('notice').textContent;
    const v=$('verification');v.hidden=!state.verification;if(state.verification){const r=state.verification;v.replaceChildren();const h=document.createElement('h2');h.textContent=r.ok?'✓ Git log verified · '+r.actual+' / '+r.expected+' commits':'Local history differs from the plan';v.append(h);[...(r.issues||[]),'Verified HEAD: '+r.head,...(state.target?['Target: '+state.target+' · '+state.pending+' commits to send']:[])].forEach(line=>{const el=document.createElement('p');el.textContent=line;v.append(el);});}
    $('session-label').textContent=initial.offline?'Offline snapshot':state.kind==='publish'?'Awaiting your approval':'Local preview';renderCalendar();renderStats();$('tooltip').hidden=true;
    if(selectedDate){const day=p.days.find(d=>d.date===selectedDate);if(day)dayDetails(day);else{selectedDate=null;$('day-title').textContent='A plan you can inspect.';$('day-description').textContent='Click any day to explore its scheduled commit timestamps.';$('day-times').replaceChildren();}}
  }
  function download(data,name,type) {const url=URL.createObjectURL(new Blob([data],{type}));const a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}
  function svgData() {
    const p=state.plan,offset=dateOf(p.days[0].date).getUTCDay(),weeks=Math.ceil((p.days.length+offset)/7),width=Math.max(800,Math.min(53,weeks)*16+100),height=Math.ceil(weeks/53)*155+140,getLevel=levels(p.days.map(d=>d.projected));
    const palette=document.documentElement.dataset.theme==='light'?['#eff2f5','#aceebb','#4ac26b','#2da44e','#116329']:['#161b22','#0e4429','#006d32','#26a641','#39d353'];const dark=document.documentElement.dataset.theme!=='light';
    let svg='<svg xmlns="http://www.w3.org/2000/svg" width="'+width+'" height="'+height+'"><rect width="100%" height="100%" fill="'+(dark?'#0d1117':'#ffffff')+'"/><g font-family="sans-serif" fill="'+(dark?'#e6edf3':'#1f2328')+'"><text x="28" y="32" font-size="19">'+p.statistics.total+' projected contributions</text><text x="28" y="55" font-size="12">'+p.config.start+' to '+p.config.end+' · Activity Forge</text>';
    const monthLabels=new Map();
    p.days.forEach((d,i)=>{const col=Math.floor((i+offset)/7),dt=dateOf(d.date),label=dt.toLocaleDateString('en-US',{month:'short',timeZone:'UTC'});if(col%53===0&&!monthLabels.has(col))monthLabels.set(col,label);if(dt.getUTCDate()===1){monthLabels.set(col,label);if(col%53===1)monthLabels.delete(col-1);}});
    monthLabels.forEach((label,col)=>{svg+='<text x="'+(70+(col%53)*16)+'" y="'+(70+Math.floor(col/53)*155)+'" font-size="11">'+label+'</text>';});
    for(let block=0;block<Math.ceil(weeks/53);block++)['','Mon','','Wed','','Fri',''].forEach((label,row)=>{if(label)svg+='<text x="28" y="'+(91+block*155+row*16)+'" font-size="11">'+label+'</text>';});
    p.days.forEach((d,i)=>{const col=Math.floor((i+offset)/7),row=(i+offset)%7,x=70+(col%53)*16,y=80+Math.floor(col/53)*155+row*16;svg+='<rect x="'+x+'" y="'+y+'" width="12" height="12" rx="2" fill="'+palette[getLevel(d.projected)]+'"><title>'+d.date+': '+d.new+' new, '+d.projected+' total</title></rect>';});
    return svg+'<text x="28" y="'+(height-28)+'" font-size="12">'+p.statistics.planned+' new commits · '+p.statistics.active_days+' active days · projected colors are estimates</text></g></svg>';
  }
  async function exportPreview() {
    const f=$('export-format').value,name='activityforge-'+state.plan.id.slice(0,10)+'.'+f;
    if(!initial.offline){const res=await fetch('/api/export?format='+f,{headers:{'X-ActivityForge-Token':initial.token}});if(!res.ok)throw new Error(await res.text());download(await res.blob(),name,res.headers.get('Content-Type'));return;}
    if(f==='json'){download(JSON.stringify(state.plan,null,2),name,'application/json');return;}
    if(f==='csv'){const rows=[['date','target','existing','new','projected','existing_known'],...state.plan.days.map(d=>[d.date,d.target,d.existing,d.new,d.projected,state.plan.existing.known])];download(rows.map(r=>r.join(',')).join('\n'),name,'text/csv');return;}
    if(f==='html'){download('<!doctype html>\n'+document.documentElement.outerHTML,name,'text/html');return;}
    const svg=svgData();if(f==='svg'){download(svg,name,'image/svg+xml');return;}
    const url=URL.createObjectURL(new Blob([svg],{type:'image/svg+xml'}));const img=new Image();img.onload=()=>{const canvas=document.createElement('canvas');canvas.width=img.width;canvas.height=img.height;canvas.getContext('2d').drawImage(img,0,0);canvas.toBlob(blob=>download(blob,name,'image/png'));URL.revokeObjectURL(url);};img.onerror=()=>{URL.revokeObjectURL(url);showError(new Error('PNG export failed'));};img.src=url;
  }
  $('settings').addEventListener('submit',e=>e.preventDefault());
  $('settings').addEventListener('input',e=>{updateOutputs();clearTimeout(timer);timer=setTimeout(()=>recalculate(e.target.id==='preset'),220);});
  $('regenerate').addEventListener('click',()=>{put('seed',crypto.getRandomValues(new Uint32Array(1))[0]);recalculate(false);});
  $('reset').addEventListener('click',()=>{config=clone(initial.plan.config);syncForm();recalculate(false);});
  document.querySelectorAll('[data-days]').forEach(el=>el.addEventListener('click',()=>{const end=dateOf($('end').value),start=new Date(end);start.setUTCDate(end.getUTCDate()-Number(el.dataset.days)+1);put('start',start.toISOString().slice(0,10));recalculate(false);}));
  document.querySelectorAll('[data-mode]').forEach(el=>el.addEventListener('click',()=>{mode=el.dataset.mode;document.querySelectorAll('[data-mode]').forEach(b=>b.classList.toggle('selected',b===el));renderCalendar();}));
  $('theme').addEventListener('click',()=>{document.documentElement.dataset.theme=document.documentElement.dataset.theme==='dark'?'light':'dark';$('theme').textContent=document.documentElement.dataset.theme==='dark'?'☀':'☾';renderCalendar();});
  async function fetchBaseline(){try{$('fetch').disabled=true;$('fetch-control').disabled=true;state=await api('fetch',{config:collect()});config=clone(state.plan.config);syncForm();render();}catch(e){showError(e);}finally{$('fetch').disabled=false;$('fetch-control').disabled=false;}}
  $('fetch').addEventListener('click',fetchBaseline);$('fetch-control').addEventListener('click',fetchBaseline);
  $('freeze').addEventListener('click',async()=>{try{clearTimeout(timer);await recalculate(false);if(!$('error').hidden)return;state=await api('freeze',{id:state.plan.id});render();}catch(e){showError(e);}});
  $('draft').addEventListener('click',async()=>{try{state=await api('draft',{});render();}catch(e){showError(e);}});
  $('back').addEventListener('click',async()=>{try{state=await api('draft',{});config=clone(state.plan.config);syncForm();render();}catch(e){showError(e);}});
  $('push').addEventListener('click',async()=>{if(!state.can_push)return;if(!confirm('Push '+state.pending+' verified commits to '+state.target+'?'+(state.notice?'\n\n'+state.notice:'')))return;try{$('push').disabled=true;state=await api('push',{id:state.plan.id,head:state.verification.head});render();}catch(e){showError(e);$('push').disabled=true;}});
  $('cancel').addEventListener('click',async()=>{try{state=await api('cancel',{});render();}catch(e){showError(e);}});
  $('export').addEventListener('click',()=>exportPreview().catch(showError));
  syncForm();render();
})();
