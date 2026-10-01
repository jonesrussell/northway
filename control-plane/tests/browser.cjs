// Real browser -> Waaseyaa accounts/session/CSRF -> Go HTTP -> SQLite.
// Disposable test credentials and invitation files only; never production.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const assert = require('node:assert/strict');
const origin=process.env.BETA_ORIGIN;
const app=process.env.BETA_APP;
const privateDir=process.env.BETA_PRIVATE_DIR;
const password='Disposable-NorthCloud-fixture-Only-49!';
function operator(input){return execFileSync('php',[app+'/bin/beta-account.php'],{input:JSON.stringify(input),env:process.env,encoding:'utf8'});}
async function request(page,path,method='GET',body=null){return await page.evaluate(async({path,method,body})=>{const token=decodeURIComponent((document.cookie.match(/(?:^|; )XSRF-TOKEN=([^;]*)/)||[])[1]||'');const response=await fetch(path,{method,headers:{'Content-Type':'application/json','X-XSRF-TOKEN':token},body:body===null?null:JSON.stringify(body)});return {status:response.status,body:await response.text()};},{path,method,body});}
(async()=>{
 const browser=await chromium.launch({headless:true,...(process.env.BETA_CHROMIUM ? {executablePath:process.env.BETA_CHROMIUM} : {})});
 try {
  const contexts=[];const pages=[];const ids=[];
  const racePath=privateDir+'/race-invite';operator({action:'invite',output_file:racePath});const raceToken=fs.readFileSync(racePath,'utf8').trim();
  const racers=await Promise.all([0,1].map(async()=>{const c=await browser.newContext();const p=await c.newPage();await p.goto(origin+'/register');return p;}));
  const race=await Promise.all(racers.map((p,i)=>request(p,'/api/auth/register','POST',{name:'Race '+i,email:'race'+i+'@example.test',password,invite_token:raceToken})));
  assert.equal(race.filter(r=>r.status===201).length,1,'Exactly one simultaneous registration may consume the invitation');assert(race.every(r=>[201,422,503].includes(r.status)));
  assert.equal((await request(racers[0],'/api/auth/register','POST',{name:'Reuse',email:'reuse@example.test',password,invite_token:raceToken})).status,422);
  for(let index=0;index<2;index++){
   const tokenPath=privateDir+'/invite-'+index;operator({action:'invite',output_file:tokenPath});const token=fs.readFileSync(tokenPath,'utf8').trim();
   const context=await browser.newContext();contexts.push(context);const page=await context.newPage();pages.push(page);await page.goto(origin+'/register');
   const response=await request(page,'/api/auth/register','POST',{name:'Fixture '+index,email:'fixture'+index+'@example.test',password,invite_token:token});assert.equal(response.status,201,response.body);const body=JSON.parse(response.body);assert.equal(body.meta.approval_required,false);assert.equal(body.meta.verification_required,false);assert.equal(body.data.email_verified,false);
   ids.push(String(body.data.id));
   await page.goto(origin+'/login');await page.getByLabel('Email',{exact:true}).fill('fixture'+index+'@example.test');await page.getByLabel('Password',{exact:true}).fill(password);await page.getByRole('button',{name:'Sign in',exact:true}).click();await page.waitForURL('**/app');
   await page.locator('#feed option').first().waitFor({state:'attached',timeout:30000});
   const denied=await page.evaluate(async()=>{const r=await fetch('/api/customer/keys',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({scopes:'feeds:read'})});return r.status;});assert.equal(denied,403,'Missing CSRF must fail');
   await page.getByText('API keys and usage limits',{exact:true}).click();await page.getByRole('button',{name:'Create API key',exact:true}).click();await page.locator('#reveal').waitFor({state:'visible'});const key=await page.locator('#secret').textContent();assert.match(key,/^nw1_/);
   const external=await context.request.get(process.env.BETA_API_ORIGIN+'/v1/feeds',{headers:{Authorization:'Bearer '+key}});assert.equal(external.status(),200);
   const list=await request(page,'/api/customer/keys');assert.equal(list.status,200);assert(!list.body.includes(key));const keyId=JSON.parse(list.body).keys[0].id;
   if(index===0){fs.writeFileSync(privateDir+'/first-key-id',keyId,{mode:0o600});}
   else{const foreign=fs.readFileSync(privateDir+'/first-key-id','utf8');const denied=await request(page,'/api/customer/keys/'+foreign,'DELETE');assert.equal(denied.status,404);assert(!list.body.includes(foreign));}
   await page.getByRole('button',{name:'Revoke',exact:true}).first().click();await page.getByText('Key revoked.',{exact:true}).waitFor();assert.equal((await context.request.get(process.env.BETA_API_ORIGIN+'/v1/feeds',{headers:{Authorization:'Bearer '+key}})).status(),401);
   // Untrusted content must be text, not inserted HTML. Feed result checks below
   // use actual publisher metadata; no mock route or seeded customer corpus.
  }
  const page=pages[0];await page.getByLabel('What are you working on?').fill('Go services and Kubernetes');
  let observed=false;
  for(let attempt=0;attempt<5;attempt++){
   await page.getByRole('button',{name:'Find relevant headlines',exact:true}).click();await page.getByRole('button',{name:'Find relevant headlines',exact:true}).waitFor({state:'visible'});
   await page.waitForTimeout(2000);if(await page.locator('#results article').count()){observed=true;break;}await page.waitForTimeout(12000);
  }
  assert(observed,'Actual approved-source metadata did not reach the reader within the bounded observation window');
  await page.getByRole('button',{name:'Save feedback',exact:true}).first().click();await page.getByRole('button',{name:'Recorded',exact:true}).first().waitFor();
  assert.equal(await page.evaluate(()=>localStorage.length),0);
  const oldContext=await browser.newContext();const oldPage=await oldContext.newPage();await oldPage.goto(origin+'/login');assert.equal((await request(oldPage,'/api/auth/login','POST',{username:'fixture0@example.test',password})).status,200);
  assert.equal((await request(oldPage,'/api/customer/keys')).status,200);
  const resetPath=privateDir+'/recovery';operator({action:'recovery',account_id:ids[0],identity_verified:true,output_file:resetPath});const resetToken=fs.readFileSync(resetPath,'utf8').trim();
  const recoveryContext=await browser.newContext();const recoveryPage=await recoveryContext.newPage();await recoveryPage.goto(origin+'/reset-password');
  const resetBody={token:resetToken,password:password+'new',password_confirmation:password+'new'};
  assert.equal((await request(recoveryPage,'/api/auth/reset-password','POST',resetBody)).status,200);
  for(const unsupported of ['/api/auth/2fa/setup','/api/auth/2fa/enable','/api/auth/2fa/verify','/api/user/me','/oauth/authorize']) {
   assert.equal((await request(oldPage,unsupported,'POST',{})).status,404,'Unsupported account surface must be unreachable even with an old session');
  }
  assert.equal((await request(recoveryPage,'/api/auth/reset-password','POST',resetBody)).status,422,'Recovery token cannot be reused');
  assert.equal((await request(page,'/api/customer/keys')).status,401,'Original session cannot mint keys after recovery');
  assert.equal((await request(oldPage,'/api/customer/keys')).status,401,'Other session cannot mint keys after recovery');
  assert.equal((await request(recoveryPage,'/api/auth/login','POST',{username:'fixture0@example.test',password:password+'new'})).status,200);
  assert.equal((await request(recoveryPage,'/api/customer/keys')).status,200);
  for(let i=0;i<2;i++){operator({action:'export',account_id:ids[i],identity_verified:true,output_file:privateDir+'/account-'+i+'.json'});assert.equal(JSON.parse(fs.readFileSync(privateDir+'/account-'+i+'.json','utf8')).email_verified,false);}
  await page.getByRole('button',{name:'Sign out',exact:true}).click();await page.waitForURL('**/login');
  console.log('PASS: two real invited accounts, concurrent single-use invitations, unverified email sign-in, sessions/CSRF, tenant isolation, one-time key reveal, independent API, revocation, live metadata reader, feedback, recovery token reuse denial, old-session invalidation, unsupported account routes denied, private account exports and logout');
 } finally {await browser.close();}
})().catch(error=>{console.error(error.message);process.exitCode=1;});
