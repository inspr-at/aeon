// SPDX-License-Identifier: AGPL-3.0-only
// Coordinator runs: npx playwright test -c playwright.ui.config.ts journey.spec.ts
// Production router remains coordinator-owned. This harness mounts the real view
// with the existing shell/session and mocks only contract HTTP routes.
import { test, expect, type Page } from '@playwright/test'
import type { Journey, ReleaseWalker, Stage, Intake } from '../src/lib/journey'
const project='10000000-0000-4000-8000-000000000001', release='20000000-0000-4000-8000-000000000001', gate='30000000-0000-4000-8000-000000000001'
const feature='40000000-0000-4000-8000-000000000001', second='40000000-0000-4000-8000-000000000002'
const id=(n:number)=>`50000000-0000-4000-8000-${String(n).padStart(12,'0')}`
const stamp='2026-09-23T10:00:00Z'
const stageKeys: Stage[]=['inspire','shape','requirements','plan','build','deploy','access','live']
function node(nodeId:string,title:string,key:string,body='Recorded description') { return {id:nodeId,key,title,body,kind_id:'kind',fields:{},state:'open',parent_id:project,position:'a',created_at:stamp,updated_at:stamp} }
async function setup(page:Page, options:{stage?:Stage;kind?:string;many?:boolean}={}) {
  const current=options.stage||'plan'
  const journey:Journey={project_node_id:project,profile:'professional',revision:5,stage:current,stages:stageKeys.map((key,i)=>({key,state:i<stageKeys.indexOf(current)?'done':key===current?'current':'later'})),next_action:{key:current==='build'?'wait_for_build':current==='requirements'?'approve_requirements':'start_build',label:current==='build'?'Building release':current==='requirements'?'Agree requirements':'Start build',stage:current,available:current!=='build',approval_request_id:gate},requirements_revision:2,current_release_id:release}
  const walker:ReleaseWalker={project_node_id:project,release_node_id:release,revision:7,state:current==='build'?'building':'planning',features:[{feature_node_id:feature,epic_key:'EP-1',title:'Order flow',selection:'some'},{feature_node_id:second,epic_key:'EP-2',title:'Customer access',selection:'none'},{feature_node_id:'empty',epic_key:'EP-3',title:'Future feature',selection:'empty'}],tickets:Array.from({length:options.many?30:4},(_,i)=>({ticket_node_id:id(i+1),key:`T-${i+1}`,title:`Ticket ${i+1}`,feature_node_id:i<2?feature:second,included:i===0,position:i,estimated_hours:2,screen_node_ids:i===0?['screen-1','screen-2']:[]}))}
  const intake:Intake={sources:[],turns:[],drafts:[]}
  const approval={id:gate,agent_principal_id:'agent',scope:'journey.build',resource_kind:'node',resource_id:release,rationale:'Build the agreed release plan.',expires_at:'2099-01-01T00:00:00Z',proposed_at:stamp,decision:'approved' as string|null,decided_by_principal_id:'person'}
  const state={journey,walker,intake,approval,failWrite:false,failRead:false,slowProject:false}
  const calls:{path:string;method:string;body:any}[]=[]
  await page.addInitScript(()=> { class MockEventSource { constructor(){} addEventListener(){} close(){} }; Object.assign(window,{EventSource:MockEventSource}) })
  await page.route('**/api/**',async route=>{
    const path=new URL(route.request().url()).pathname,method=route.request().method(),body=route.request().postDataJSON()
    calls.push({path,method,body})
    if(path==='/api/me') return route.fulfill({json:{principal:{id:'person',kind:options.kind||'person',name:'Markus Barta'},tenant:{id:'tenant',name:'INSPR'}}})
    if(path==='/api/version') return route.fulfill({json:{version:'260923161128.0.0',scheme:'inspr-calendar-v2'}})
    if(state.failWrite && method!=='GET') return route.fulfill({status:409,json:{error:'Stale revision'}})
    if(state.failRead && path.includes('/journey')) return route.fulfill({status:503,json:{error:'Journey unavailable'}})
    if(path===`/api/projects/${project}/journey`) return route.fulfill({json:state.journey})
    if(path.endsWith('/journey/profile')) { state.journey.profile=body.profile;state.journey.revision++;return route.fulfill({json:state.journey}) }
    if(path.endsWith('/journey/actions')) { state.journey.revision++;state.journey.stage='build';state.journey.stages=stageKeys.map((key,i)=>({key,state:i<4?'done':i===4?'current':'later'}));state.journey.next_action={key:'wait_for_build',label:'Building release',stage:'build',available:false};state.walker.state='building';return route.fulfill({json:state.journey}) }
    if(path.endsWith('/requirements/agree')) return route.fulfill({json:[]})
    if(path.endsWith('/requirements')) return route.fulfill({json:method==='POST'?{...body,node_id:'req',project_node_id:project,status:'draft',revision:3}:[{node_id:'req',project_node_id:project,kind:'functional',revision:2,status:'agreed',title:'Take orders',feature_node_id:feature,generated_ticket_ids:[id(1),id(2)]}]})
    if(path.endsWith('/walker')) return route.fulfill({json:state.walker})
    if(path.endsWith('/plan')) { state.walker.tickets=body.ordered_ticket_ids.map((ticketId:string,index:number)=>({...state.walker.tickets.find(t=>t.ticket_node_id===ticketId),position:index,included:body.included_ticket_ids.includes(ticketId)}));state.walker.revision++;return route.fulfill({json:state.walker}) }
    if(path.endsWith('/intake')) return route.fulfill({json:state.intake})
    if(path.endsWith('/accept')) return route.fulfill({json:state.intake.drafts[0]})
    if(path==='/api/approvals') return route.fulfill({json:[state.approval]})
    if(path.endsWith('/decision')) { state.approval.decision=body.decision;return route.fulfill({json:state.approval}) }
    if(path.startsWith('/api/nodes/')) {
      const nodeId=decodeURIComponent(path.split('/').at(-1)!)
      if(nodeId==='tree') return route.fulfill({json:{items:[],next_cursor:null}})
      if(nodeId===project) return route.fulfill({json:node(project,'Bakery orders','PROJ-1')})
      if(nodeId===release) return route.fulfill({json:node(release,'Release 1','REL-1')})
      const t=state.walker.tickets.find(t=>t.ticket_node_id===nodeId)
      return route.fulfill({json:node(nodeId,t?.title||'Linked screen',t?.key||nodeId,nodeId==='screen-1'?'# Order form\nChoose your bread.':nodeId==='screen-2'?'# Revised form\nChoose a pickup time.':'Recorded ticket evidence')})
    }
    if(path==='/api/kinds'||path==='/api/views') return route.fulfill({json:{items:[]}})
    if(path.startsWith('/api/stage-handoffs/')) return route.fulfill({json:{id:'handoff',project_node_id:project,release_node_id:release,stage:'deploy',operation:'deploy',plugin_id:'pharos',attempt:2,authority_epoch:2,state:'blocked',expires_at:'2099-01-01T00:00:00Z',result:{outcome:'failed',blocker_code:'policy_refused',completed_at:stamp}}})
    return route.fulfill({status:404,json:{error:'Uncontracted route'}})
  })
  return {state,calls}
}
async function mount(page:Page,stage?:Stage) {
  await page.goto('/')
  await page.evaluate(async ({project,stage})=>{
    // Dynamic import strings deliberately refer to Vite's served source modules.
    const routerModule='/src/router.ts',viewModule='/src/views/JourneyView.vue'
    const {router}=await import(routerModule),{default:component}=await import(viewModule)
    router.addRoute({path:'/projects/:projectId',component})
    router.addRoute({path:'/projects/:projectId/journey/:stage',component})
    await router.push(`/projects/${project}${stage?`/journey/${stage}`:''}`)
  },{project,stage})
  await expect(page.getByRole('navigation',{name:'Project journey'})).toBeVisible()
}
async function walker(page:Page) { await page.getByRole('button',{name:'Full screen',exact:true}).click();return page.getByRole('dialog',{name:'Release walker',exact:true}) }

test.use({viewport:{width:1280,height:720}})
test('server default stage, eight steps, fixed workspace, no hover shift',async({page})=>{
  await setup(page);await mount(page)
  await expect(page).toHaveURL(new RegExp(`/projects/${project}/journey/plan$`))
  await expect(page.getByRole('navigation',{name:'Project journey'}).locator('li')).toHaveCount(8)
  await expect(page.getByText('No tickets · no accepted breakdown yet.')).toBeVisible()
  const button=page.getByRole('button',{name:'Full screen',exact:true}),before=await button.boundingBox();await button.hover();expect(await button.boundingBox()).toEqual(before)
  expect(await page.evaluate(()=>({body:document.documentElement.scrollHeight<=innerHeight,main:document.querySelector('main')!.scrollHeight<=document.querySelector('main')!.clientHeight}))).toEqual({body:true,main:true})
})
test('A3 feature checkbox restores its remembered partial set with revision-fenced PUTs',async({page})=>{
  const {calls}=await setup(page);await mount(page);const dialog=await walker(page)
  const group=dialog.getByRole('checkbox',{name:/Select Order flow/})
  await expect(group).toHaveAttribute('aria-checked','mixed')
  for(const value of ['true','false','mixed']) {await group.click();await expect(group).toHaveAttribute('aria-checked',value);await expect(group).toBeEnabled()}
  const writes=calls.filter(c=>c.method==='PUT'&&c.path.endsWith('/plan'))
  expect(writes.map(c=>c.body.expected_revision)).toEqual([7,8,9])
  expect(writes.map(c=>c.body.included_ticket_ids)).toEqual([[id(1),id(2)],[],[id(1)]])
  expect(writes.every(c=>c.body.ordered_ticket_ids.length===4)).toBeTruthy()
})
test('A3 wraps tickets and features; search, screens, compare, zoom, details and focus',async({page})=>{
  await setup(page);await mount(page);const dialog=await walker(page)
  await dialog.focus();await page.keyboard.press('ArrowLeft');await expect(dialog.getByRole('heading',{name:'Ticket 4',exact:true})).toBeVisible()
  await page.keyboard.press('Shift+ArrowRight');await expect(dialog.getByRole('heading',{name:'Ticket 1',exact:true})).toBeVisible()
  await expect(dialog.getByRole('heading',{name:'Order form',exact:true})).toBeVisible()
  await page.keyboard.press('ArrowDown');await expect(dialog.getByRole('heading',{name:'Revised form',exact:true})).toBeVisible()
  await page.keyboard.press('c');await expect(dialog.getByLabel('Compare linked screen')).toBeVisible()
  await page.keyboard.press('z');await expect(dialog.getByRole('button',{name:'Zoom to 100 percent'})).toHaveAttribute('aria-pressed','true')
  await page.keyboard.press('i');await expect(dialog.getByLabel('Ticket details', {exact:true})).toHaveCount(1)
  await page.keyboard.press('/');await dialog.getByLabel('Search tickets').fill('Ticket 3');await page.keyboard.press('Enter')
  await page.keyboard.press('i');await expect(dialog.getByRole('heading',{name:'Ticket 3',exact:true})).toBeVisible()
  await dialog.focus();await page.keyboard.press('?');await expect(dialog.getByRole('heading',{name:'Shortcuts'})).toBeVisible()
  await page.keyboard.press('Escape');await expect(dialog).toBeVisible();await page.keyboard.press('Escape');await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('button',{name:'Full screen',exact:true})).toBeFocused()
})
test('dragging the A3 header never activates a ticket',async({page})=>{
  await setup(page,{many:true});await mount(page);const dialog=await walker(page)
  const chip=dialog.locator('.ticket-chip button').first(),box=await chip.boundingBox();expect(box).not.toBeNull()
  await page.mouse.move(box!.x+box!.width/2,box!.y+box!.height/2);await page.mouse.down();await page.mouse.move(box!.x-220,box!.y+box!.height/2,{steps:12});await page.mouse.up()
  await expect(dialog.getByRole('heading',{name:'Ticket 1',exact:true})).toBeVisible()
  expect(await dialog.locator('.walker-rail').evaluate(el=>el.scrollLeft)).toBeGreaterThan(0)
})
test('conflicting plans preserve server selection and block writes until refresh',async({page})=>{
  const {state,calls}=await setup(page);await mount(page);const dialog=await walker(page);state.failWrite=true
  await dialog.getByRole('checkbox',{name:'Include T-2',exact:true}).click()
  await expect(dialog.getByRole('alert')).toContainText('This project changed')
  await expect(dialog.getByRole('checkbox',{name:'Include T-2',exact:true})).not.toBeChecked()
  await expect(dialog.getByRole('checkbox',{name:'Include T-1',exact:true})).toBeDisabled()
  expect(calls.filter(c=>c.method==='PUT').length).toBe(1)
  state.failWrite=false;await dialog.getByRole('button',{name:'Refresh',exact:true}).click();await expect(dialog.getByRole('checkbox',{name:'Include T-1',exact:true})).toBeEnabled()
})
test('one server next action, bounded build approval, then passive build with no plan editing',async({page})=>{
  const {calls}=await setup(page);await mount(page)
  await page.getByRole('button',{name:'Start build',exact:true}).click();await page.getByRole('button',{name:'Confirm action',exact:true}).click()
  await expect(page).toHaveURL(/journey\/build$/)
  await expect(page.getByRole('navigation',{name:'Project journey'})).toContainText('Building release')
  expect(calls.find(c=>c.path.endsWith('/journey/actions'))?.body).toMatchObject({action:'start_build',expected_revision:5,approval_request_id:gate,release_id:release})
  expect(calls.filter(c=>c.path.endsWith('/decision'))).toHaveLength(0)
  const dialog=await walker(page);await expect(dialog.getByRole('checkbox',{name:'Include T-1',exact:true})).toBeDisabled()
  await dialog.focus();await page.keyboard.press('Space');expect(calls.filter(c=>c.path.endsWith('/plan'))).toHaveLength(0)
})
test('profile writes use stable slugs and keep gate history',async({page})=>{
  const {calls}=await setup(page);await mount(page)
  const profile=page.getByLabel('Profile',{exact:true})
  await expect(profile).toHaveAccessibleName('Profile')
  await expect(profile).toHaveValue('professional')
  await profile.selectOption('enterprise')
  await expect(page.getByRole('status')).toContainText('Profile saved')
  await expect(profile).toHaveValue('enterprise')
  expect(calls.find(c=>c.path.endsWith('/journey/profile'))?.body).toEqual({profile:'enterprise',expected_revision:5})
  await expect(page.getByLabel('Human gate')).toContainText('approved')
})
test('agent and unavailable action cannot mutate the journey',async({page})=>{
  const {state,calls}=await setup(page,{kind:'agent'});state.journey.next_action.available=false;state.journey.next_action.reason='Requirements agreement expired.'
  await mount(page)
  await expect(page.getByRole('button',{name:'Start build',exact:true})).toBeDisabled()
  await expect(page.getByLabel('Profile',{exact:true})).toBeDisabled()
  const dialog=await walker(page);await expect(dialog.getByRole('checkbox',{name:'Include T-1',exact:true})).toBeDisabled()
  expect(calls.filter(c=>c.method!=='GET')).toHaveLength(0)
})
test('human gate decisions are explicit R2 writes, distinct from journey actions',async({page})=>{
  const {state,calls}=await setup(page);state.approval.decision=null;state.journey.next_action.available=false;state.journey.next_action.reason='Build gate pending.'
  await mount(page);await page.getByLabel('Decision reason').fill('Reviewed scope and budget');await page.getByRole('button',{name:'Approve gate',exact:true}).click()
  await expect(page.getByLabel('Human gate')).toContainText('approved')
  expect(calls.find(c=>c.path.endsWith('/decision'))?.body).toEqual({decision:'approved',reason:'Reviewed scope and budget'})
  expect(calls.filter(c=>c.path.endsWith('/journey/actions'))).toHaveLength(0)
})
test('requirements agreement uses its dedicated endpoint and project revision',async({page})=>{
  const {calls}=await setup(page,{stage:'requirements'});await mount(page)
  await page.getByRole('button',{name:'Agree requirements',exact:true}).click();await page.getByRole('button',{name:'Confirm action'}).click()
  await expect(page.getByRole('status')).toContainText('Requirements agreed')
  expect(calls.find(c=>c.path.endsWith('/requirements/agree'))?.body).toMatchObject({expected_revision:5,approval_request_id:gate})
  expect(calls.filter(c=>c.path.endsWith('/journey/actions'))).toHaveLength(0)
})
test('deployment blocker and skipped access reflect server evidence',async({page})=>{
  const {state}=await setup(page,{stage:'deploy'});state.journey.next_action={key:'retry_deploy',label:'Retry deployment',stage:'deploy',available:false,reason:'Fresh backup evidence required.'};state.journey.stages.find(s=>s.key==='deploy')!.handoff_id='handoff';state.journey.stages.find(s=>s.key==='access')!.state='skipped'
  await mount(page);await expect(page.getByText('policy refused',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Retry deployment'})).toBeDisabled()
  await page.getByRole('link',{name:'Access · skipped',exact:true}).click();await expect(page.getByRole('heading',{name:'Not needed in this release'})).toBeVisible()
})
test('intake shows saved citations and submits base-event-fenced acceptance',async({page})=>{
  const {state,calls}=await setup(page,{stage:'inspire'});state.journey.next_action={key:'continue_intake',label:'Continue conversation',stage:'inspire',available:true}
  state.intake.sources=[{id:'source',project_node_id:project,kind:'conversation',label:'Bakery conversation',content_sha256:'a'.repeat(64),created_at:stamp}]
  state.intake.turns=[{id:'turn',source_id:'source',ordinal:0,speaker:'person',body:'Make bread orders easy.',created_at:stamp}]
  state.intake.drafts=[{id:'draft',kind:'brief',title:'Order brief',body:'A short order form.',base_event_id:42,status:'proposed',proposed_at:stamp,citations:[{source_id:'source',turn_id:'turn',locator:'turn 1'}]}]
  await mount(page);await page.getByRole('button',{name:'Transcript',exact:true}).click();await expect(page.getByText('Make bread orders easy.')).toBeVisible()
  await expect(page.getByText('Bakery conversation · turn 1')).toBeVisible();await page.getByRole('button',{name:'Accept draft',exact:true}).click()
  await expect(page.getByRole('status')).toContainText('Cited draft accepted');expect(calls.find(c=>c.path.endsWith('/accept'))?.body).toEqual({expected_base_event_id:42})
})

test('expired human gates are readable but cannot be approved',async({page})=>{
  const {state,calls}=await setup(page);state.approval.decision=null;state.approval.expires_at='2000-01-01T00:00:00Z';state.journey.next_action.available=false
  await mount(page);await expect(page.getByLabel('Human gate')).toContainText('Expired')
  await expect(page.getByRole('button',{name:'Approve gate',exact:true})).toHaveCount(0)
  expect(calls.filter(c=>c.method!=='GET')).toHaveLength(0)
})
test('failed journey read has an explicit retry and never invents a stage',async({page})=>{
  const {state}=await setup(page);state.failRead=true
  await page.goto('/')
  await page.evaluate(async project=>{
    const routerModule='/src/router.ts',viewModule='/src/views/JourneyView.vue'
    const {router}=await import(routerModule),{default:component}=await import(viewModule)
    router.addRoute({path:'/projects/:projectId/journey/:stage',component})
    await router.push(`/projects/${project}/journey/plan`)
  },project)
  await expect(page.getByRole('heading',{name:'Journey unavailable'})).toBeVisible()
  await expect(page.getByRole('navigation',{name:'Project journey'})).toHaveCount(0)
  state.failRead=false;await page.getByRole('button',{name:'Refresh',exact:true}).click()
  await expect(page.getByRole('navigation',{name:'Project journey'})).toBeVisible()
})
test('empty walker keeps empty features without synthetic tickets or screenshots',async({page})=>{
  const {state,calls}=await setup(page);state.walker.tickets=[]
  await mount(page);const dialog=await walker(page)
  await expect(dialog.getByRole('heading',{name:'No tickets yet'})).toBeVisible()
  await expect(dialog.locator('.no-tickets')).toHaveCount(3)
  await expect(dialog.getByRole('button',{name:'Next ticket',exact:true})).toBeDisabled()
  await dialog.focus();await page.keyboard.press('ArrowRight');await page.keyboard.press('Space')
  expect(calls.filter(c=>c.method!=='GET')).toHaveLength(0)
})

test('leaving the journey preserves R1 node details and R2 human decisions',async({page})=>{
  const {state,calls}=await setup(page)
  state.approval.decision=null
  const projectNode=node(project,'Bakery orders','PROJ-1','# Order context\n\nA **shared** plan.')
  await page.route('**/api/nodes/tree',route=>route.fulfill({json:{items:[{node:projectNode,depth:0}],next_cursor:null}}))
  await page.route(`**/api/nodes/${project}`,route=>route.fulfill({json:projectNode}))
  await mount(page)
  await page.getByRole('link',{name:'PAIMOS AEON home',exact:true}).click()
  await expect(page.getByRole('navigation',{name:'Project journey'})).toHaveCount(0)
  await expect(page.getByRole('heading',{name:'Projects',level:1})).toBeVisible()
  await page.evaluate(async()=>{const routerModule='/src/router.ts';const {router}=await import(routerModule);await router.push('/workspace')})
  await page.getByRole('button',{name:'Open PROJ-1: Bakery orders',exact:true}).click()
  const details=page.getByRole('complementary',{name:'Node details'})
  await expect(details.getByRole('heading',{name:'Order context',exact:true})).toBeVisible()
  await expect(details.locator('.markdown-body strong')).toHaveText('shared')
  await page.evaluate(async()=>{
    const routerModule='/src/router.ts'
    const {router}=await import(routerModule)
    await router.push('/approvals')
  })
  // The old approvals page now lives in Agents.
  await expect(page).toHaveURL('/agents')
  const request=page.getByRole('listitem',{name:/^Build journey, asked by/})
  await request.getByRole('button',{name:'Approve',exact:true}).click()
  await request.getByLabel('Reason (optional)',{exact:true}).fill('Reviewed after leaving the journey')
  await request.getByRole('button',{name:'Approve permission',exact:true}).click()
  await expect(page.locator('.toast').filter({hasText:'Approved:'})).toBeVisible()
  expect(calls.find(c=>c.path.endsWith('/decision'))?.body).toEqual({decision:'approved',reason:'Reviewed after leaving the journey'})
  expect(calls.filter(c=>c.path.endsWith('/journey/actions'))).toHaveLength(0)
})
