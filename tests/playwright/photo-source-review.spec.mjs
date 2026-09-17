import {expect,test} from '@playwright/test';
import {startBearStack,stopBearStack,freePort} from './server-fixture.mjs';
import http from 'node:http';
import {mkdtemp,mkdir,writeFile,readFile,appendFile,rm} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

const model='yunet-2023mar-sface-2021dec-v1';
let root,baseURL,app,service;
test.beforeAll(async()=>{
 root=await mkdtemp(path.join(os.tmpdir(),'bearstack-source-review-'));
 const photos=path.join(root,'photos');await mkdir(path.join(photos,'album'),{recursive:true});
 await writeFile(path.join(photos,'album/a.png'),await readFile(new URL('../../services/faces/tests/fixtures/astronaut.png',import.meta.url)));
 service=http.createServer((req,res)=>{res.setHeader('Content-Type','application/json');if(req.url==='/health'){res.end(JSON.stringify({ready:true,protocol:1,model}));return;}req.resume();req.on('end',()=>{const embedding=Array(128).fill(0);embedding[0]=1;res.end(JSON.stringify({model,faces:[{x:.1,y:.1,width:.3,height:.3,confidence:.99,embedding}]}));});});
 await new Promise(resolve=>service.listen(0,'127.0.0.1',resolve));
 const port=await freePort();baseURL=`http://127.0.0.1:${port}`;
 const configPath=path.join(root,'config.json');await writeFile(configPath,JSON.stringify({addr:`127.0.0.1:${port}`,data_dir:path.join(root,'data'),auth:{credentials:[{username:'manager',password:'secret',role:'photos_manager'}]},photos:{enabled:true,root_dir:photos,face_service_url:`http://127.0.0.1:${service.address().port}`,face_service_token:'bearstack-review-test-token-0000000'}}));
 app=await startBearStack({configPath,baseURL},{username:'manager',password:'secret'});
});
test.afterAll(async({},info)=>{info.setTimeout(75000);await stopBearStack(app);if(service)await new Promise(resolve=>service.close(resolve));if(root)await rm(root,{recursive:true,force:true});});

test('preserved preview and current source can be reviewed on desktop and mobile',async({browser})=>{
 const context=await browser.newContext({httpCredentials:{username:'manager',password:'secret'}});
 try{
  const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto(baseURL+'/login');await page.getByLabel('Benutzername').fill('manager');await page.locator('input[name="password"]').fill('secret');await page.getByRole('button',{name:'Anmelden',exact:true}).click();
  const response=await context.request.post(baseURL+'/photos/faces/analyze',{form:{path:'album/a.png'},headers:{Origin:baseURL,Accept:'application/json'}});expect(response.ok(),await response.text()).toBe(true);
  const faces=(await response.json()).photo.faces;expect(faces).toHaveLength(1);const id=faces[0].id;
  const preview=await context.request.get(`${baseURL}/photos/faces/${id}/thumbnail`);expect(preview.ok()).toBe(true);const before=await preview.body();
  await appendFile(path.join(root,'photos/album/a.png'),Buffer.from('changed-original'));
  await page.goto(`${baseURL}/photos/faces/${id}/review`);
  await expect(page.getByText('Das Foto wurde geändert.',{exact:false})).toBeVisible();
  await expect(page.locator('[data-review-stage] img')).toBeVisible();
  expect(await (await context.request.get(`${baseURL}/photos/faces/${id}/thumbnail`)).body()).toEqual(before);
  for(const width of [1440,390]){await page.setViewportSize({width,height:900});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);}
  await page.locator('input[name="name"]').fill('Ada');await page.locator('input[name="x"]').fill('0.12');
  await expect(page.locator('[data-review-box]')).toHaveCSS('left',/.+/);
  await page.getByRole('button',{name:'Rahmen und Person bestätigen'}).click();
  await expect(page).toHaveURL(/\/photos\/faces\/review/);await expect(page.getByText('Keine Gesichter zu prüfen.')).toBeVisible();
  const checked=await context.request.get(`${baseURL}/photos/faces/${id}/review`,{headers:{Accept:'application/json'}});const state=await checked.json();expect(state.face.needs_review).toBe(false);expect(state.face.name).toBe('Ada');expect(state.face.x).toBe(.12);
  await page.goto(baseURL+'/settings/photos/identities');await expect(page.getByRole('heading',{name:'Fotoordner und Aufbewahrung'})).toBeVisible();expect(errors).toEqual([]);
 }finally{await context.close();}
});
