import { expect, test } from "@playwright/test";
import { spawn } from "node:child_process";
import http from "node:http";
import net from "node:net";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-test-service-token-000000";
const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
let root, baseURL, app, service, output = "";
async function port() { const server = net.createServer(); await new Promise(resolve => server.listen(0, "127.0.0.1", resolve)); const result=server.address().port; await new Promise(resolve=>server.close(resolve));return result; }

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-faces-e2e-"));
  const photos=path.join(root,"photos");await mkdir(photos);for(const name of ["a.png","b.png","c.png"]) await writeFile(path.join(photos,name),png);
  let calls=0;
  service=http.createServer((request,response)=>{
    if(request.headers.authorization!=="Bearer "+token){response.writeHead(401);response.end();return;}
    response.setHeader("Content-Type","application/json");
    if(request.url==="/health"){response.end(JSON.stringify({ready:true,protocol:1,model}));return;}
    request.resume();request.on("end",()=>{const embedding=Array(128).fill(0);embedding[calls++===2?1:0]=1;response.end(JSON.stringify({model,faces:[{x:.1,y:.1,width:.5,height:.5,confidence:.99,embedding}]}));});
  });
  await new Promise(resolve=>service.listen(0,"127.0.0.1",resolve));
  const appPort=await port();baseURL=`http://127.0.0.1:${appPort}`;
  const config=path.join(root,"config.json");await writeFile(config,JSON.stringify({addr:`127.0.0.1:${appPort}`,data_dir:path.join(root,"data"),auth:{credentials:[{username:"manager",password:"secret",role:"photos_manager"},{username:"reader",password:"secret",role:"photos_read"}]},photos:{enabled:true,root_dir:photos,face_service_url:`http://127.0.0.1:${service.address().port}`,face_service_token:token}}));
  app=spawn("go",["run","./cmd/bearstack"],{env:{...process.env,BEARSTACK_CONFIG:config},stdio:["ignore","pipe","pipe"],detached:true});
  app.stdout.on("data",b=>{output+=b;});app.stderr.on("data",b=>{output+=b;});
  await expect.poll(async()=>{try{return (await fetch(baseURL+"/healthz",{headers:{authorization:"Basic "+Buffer.from("manager:secret").toString("base64")}})).status;}catch{return 0;}},{timeout:120000,message:()=>output}).toBe(200);
});
test.afterAll(async()=>{if(app?.pid){try{process.kill(-app.pid,"SIGTERM");}catch{}}if(service)await new Promise(resolve=>service.close(resolve));if(root)await rm(root,{recursive:true,force:true});});

test("face recognition: enable, name, move, merge, ignore and search",async({browser})=>{
  const context=await browser.newContext({httpCredentials:{username:"manager",password:"secret"}});const page=await context.newPage();
  await page.goto(baseURL+"/login");
  await page.getByLabel("Benutzername").fill("manager");await page.locator('input[name="password"]').fill("secret");await page.getByRole("button",{name:"Anmelden"}).click();
  await page.goto(baseURL+"/settings/photos/faces");
  await expect(page.getByLabel("Gesichtserkennung aktivieren")).not.toBeChecked();
  await page.getByLabel("Gesichtserkennung aktivieren").check();
  await page.getByLabel("Pause zwischen Bildern (ms)").fill("100");
  await page.getByRole("button",{name:"Speichern",exact:true}).click();
  await expect.poll(async()=>{const r=await context.request.get(baseURL+"/settings/photos/faces?format=json");return (await r.json()).status.done;}).toBe(3);
  await page.goto(baseURL+"/photos/people");await expect(page.locator("a.person-card")).toHaveCount(2);
  await page.locator("a.person-card").filter({hasText:"2 Fotos"}).click();
  await page.getByLabel("Name",{exact:true}).fill("Jürgen");await page.getByRole("button",{name:"Benennen",exact:true}).click();await expect(page.getByRole("heading",{name:"Jürgen",exact:true})).toBeVisible();
  const response=await context.request.get(baseURL+"/photos/frame/items?q=person%3AJuergen");expect((await response.json()).total).toBe(2);
  await page.getByLabel("Auswählen",{exact:true}).first().check();await page.getByLabel("Name der neuen Person").fill("Marie");await page.getByRole("button",{name:"Auswahl verschieben"}).click();
  await page.locator("a.person-card").filter({hasText:"Marie"}).click();
  await expect(page.getByLabel("Zusammenführen mit").locator("option").filter({hasText:"Jürgen"})).toHaveCount(1);
  await page.getByLabel("Zusammenführen mit").selectOption({label:await page.getByLabel("Zusammenführen mit").locator("option").filter({hasText:"Jürgen"}).textContent()});
  await page.getByRole("button",{name:"Gruppen zusammenführen"}).click();await expect(page.getByLabel("Auswählen",{exact:true})).toHaveCount(2);
  await page.getByLabel("Auswählen",{exact:true}).first().check();await page.getByRole("button",{name:"Auswahl ignorieren"}).click();
  await expect(page.locator("a.person-card").filter({hasText:"Jürgen"})).toContainText("1 Foto");
  await page.screenshot({path:"/tmp/bearstack-people.png",fullPage:true});
  await page.setViewportSize({width:390,height:844});
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
  await page.locator("a.person-card").filter({hasText:"Jürgen"}).click();
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
  await page.screenshot({path:"/tmp/bearstack-person-mobile.png",fullPage:true});
  const reader=await browser.newContext({httpCredentials:{username:"reader",password:"secret"}});const denied=await reader.request.post(baseURL+"/photos/faces/edit",{form:{face_id:"1",action:"ignore"}});expect(denied.status()).toBe(403);await reader.close();
  await page.goto(baseURL+"/settings/photos/faces");await page.getByRole("button",{name:"Pausieren",exact:true}).click();await expect(page.getByLabel("Gesichtserkennung aktivieren")).not.toBeChecked();await context.close();
});
