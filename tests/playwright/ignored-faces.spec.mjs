import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-test-service-token-000000";
const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
let root, baseURL, app, service;

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-faces-e2e-"));
  const photos = path.join(root, "photos");
  for (const name of ["a", "b", "c"]) {
    const file = path.join(photos, "2002", "20021010-Assisi", "BILDER_LUKAS", name + "-sehr-langer-bilddateiname-ohne-kurze-abstaende-012345678901234567890123456789.png");
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, png);
  }
  let calls=0;
  service=http.createServer((request,response)=>{
    if(request.headers.authorization!=="Bearer "+token){response.writeHead(401);response.end();return;}
    response.setHeader("Content-Type","application/json");
    if(request.url==="/health"){response.end(JSON.stringify({ready:true,protocol:1,model}));return;}
    request.resume();request.on("end",()=>{
      const landscape=calls++===2;
      const embedding=Array(128).fill(0);embedding[landscape?1:0]=1;
      const bounds=landscape?{x:.3,y:.1,width:.4,height:.2}:{x:.35,y:.05,width:.25,height:.5};
      response.end(JSON.stringify({model,faces:[{...bounds,confidence:.99,embedding}]}));
    });
  });
  await new Promise(resolve=>service.listen(0,"127.0.0.1",resolve));
  const appPort=await freePort();baseURL=`http://127.0.0.1:${appPort}`;
  const config=path.join(root,"config.json");await writeFile(config,JSON.stringify({addr:`127.0.0.1:${appPort}`,data_dir:path.join(root,"data"),auth:{credentials:[{username:"admin",password:"secret",role:"admin"},{username:"manager",password:"secret",role:"photos_manager"},{username:"reader",password:"secret",role:"photos_read"}]},photos:{enabled:true,root_dir:photos,face_service_url:`http://127.0.0.1:${service.address().port}`,face_service_token:token}}));
  app = await startBearStack({ configPath: config, baseURL }, { username: "manager", password: "secret" });
});
test.afterAll(async ({}, testInfo) => {
  testInfo.setTimeout(75_000);
  await stopBearStack(app);
  if (service) await new Promise(resolve => service.close(resolve));
  if (root) await rm(root, { recursive: true, force: true });
});

test("ignored portraits restore unnamed or via the naming dialog without changing siblings", async ({browser}) => {
  const context=await browser.newContext({httpCredentials:{username:"manager",password:"secret"}});
  try {
    const page=await context.newPage(), errors=[];
    page.on("pageerror",error=>errors.push(error.message));
    await page.goto(baseURL+"/login");
    await page.getByLabel("Benutzername").fill("manager");await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button",{name:"Anmelden",exact:true}).click();
    const post=async (route,form) => {
      const response=await context.request.post(baseURL+route,{form,headers:{Origin:baseURL,Accept:"application/json"}});
      expect(response.ok()).toBe(true);return response;
    };
    await post("/settings/photos/faces",{enabled:"1",delay_millis:"100"});
    await expect.poll(async()=>(await(await context.request.get(baseURL+"/settings/photos/faces?format=json")).json()).status.done).toBe(3);
    const people=(await(await context.request.get(baseURL+"/photos/people?format=json")).json()).people;
    const source=people.find(p=>p.count===2),target=people.find(p=>p.count===1);
    await post(`/photos/people/${source.id}/rename`,{name:"Altname"});
    await post(`/photos/people/${target.id}/rename`,{name:"Ziel"});
    const faces=(await(await context.request.get(baseURL+`/photos/people/${source.id}?format=json`)).json()).faces;
    const ignore=async face=>post("/photos/faces/edit",{action:"ignore",face_id:String(face)});
    const getFace=async face=>{
      const path=faces.find(f=>f.id===face).path;
      return (await(await context.request.get(baseURL+"/photos/faces?path="+encodeURIComponent(path))).json()).photo.faces.find(f=>f.id===face);
    };
    await ignore(faces[0].id);await ignore(faces[1].id);
    await page.goto(baseURL+"/photos/people?ignored=1&q=Altname&known=1&page=1");
    const cards=page.locator("[data-ignored-face]"),dialog=page.locator("[data-person-dialog]");
    await expect(cards).toHaveCount(2);await expect(dialog).toHaveCount(1);
    const firstID=Number(await cards.first().getAttribute("data-ignored-face"));
    const secondID=Number(await cards.nth(1).getAttribute("data-ignored-face"));
    for(const width of [390,1440]) {
      await page.setViewportSize({width,height:900});
      expect(await page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
      await page.screenshot({path:`/tmp/bearstack-ignored-${width}.png`,fullPage:true});
    }
    const retained=await cards.first().locator("img").elementHandle();
    await cards.nth(1).getByRole("button",{name:"Wiederherstellen",exact:true}).click();
    await expect(cards).toHaveCount(1);
    expect(await retained.evaluate(img=>img.isConnected)).toBe(true);
    expect((await getFace(secondID)).name).toBe("");expect((await getFace(secondID)).ignored).toBe(false);
    expect((await getFace(firstID)).ignored).toBe(true);
    await expect(page).toHaveURL(/ignored=1.*q=Altname.*known=1/);
    let writes=0;page.on("request",request=>{if(request.method()==="POST"&&request.url().endsWith("/photos/faces/edit"))writes++;});
    const pencil=cards.locator("[data-ignored-edit]");
    await pencil.click();
    await expect(dialog.locator("[data-person-preview-image]")).toHaveAttribute("src",new RegExp("/"+firstID+"$"));
    await expect(dialog.locator("[data-person-dialog-ignore]")).toBeHidden();
    await expect(dialog.locator("[data-person-face-match]")).toBeHidden();
    await dialog.getByRole("button",{name:"Abbrechen",exact:true}).click();
    expect(writes).toBe(0);await expect(pencil).toBeFocused();
    await pencil.click();
    await dialog.getByRole("combobox").fill("");
    await dialog.getByRole("button",{name:"Benennen und wiederherstellen",exact:true}).click();
    expect(await dialog.getByRole("combobox").evaluate(input=>input.checkValidity())).toBe(false);
    expect(writes).toBe(0);
    await dialog.getByRole("combobox").fill("Petra");
    await dialog.getByRole("button",{name:"Benennen und wiederherstellen",exact:true}).click();
    await expect(dialog).not.toBeVisible();await expect(cards).toHaveCount(0);
    expect((await getFace(firstID)).name).toBe("Petra");expect((await getFace(firstID)).ignored).toBe(false);
    expect(writes).toBe(1);
    // Restore by assigning an existing person; no group-wide merge is allowed.
    await ignore(firstID);await ignore(secondID);
    await page.goto(baseURL+"/photos/people?ignored=1&page=999&q=");
    await expect(cards).toHaveCount(2);
    await page.locator(`[data-ignored-face="${firstID}"] [data-ignored-edit]`).click();
    await dialog.getByRole("combobox").fill("Ziel");
    await dialog.getByRole("option").filter({hasNotText:"Neu anlegen:"}).click();
    await expect(dialog).not.toBeVisible();await expect(cards).toHaveCount(1);
    expect((await getFace(firstID)).person_id).toBe(target.id);
    expect((await getFace(secondID)).ignored).toBe(true);
    await expect(page).toHaveURL(/page=1/);
    // A committed write with a lost response cannot be submitted again.
    let attempts=0;
    await page.route("**/photos/faces/edit",async route=>{attempts++;await route.fetch();await route.abort();});
    await cards.getByRole("button",{name:"Wiederherstellen",exact:true}).click();
    await expect(page.locator("[data-ignored-retry]")).toBeVisible();
    await expect(cards.getByRole("button",{name:"Wiederherstellen",exact:true})).toBeDisabled();
    await page.locator("[data-ignored-retry]").click();await expect(cards).toHaveCount(0);
    expect(attempts).toBe(1);await page.unroute("**/photos/faces/edit");
    // The direct unnamed restore also works without JavaScript.
    await ignore(secondID);
    const plain=await browser.newContext({javaScriptEnabled:false,storageState:await context.storageState()});
    const plainPage=await plain.newPage();await plainPage.goto(baseURL+"/photos/people?ignored=1");
    await plainPage.getByRole("button",{name:"Wiederherstellen",exact:true}).click();
    await expect(plainPage).toHaveURL(/ignored=1/);await expect(plainPage.locator("[data-ignored-face]")).toHaveCount(0);
    expect((await getFace(secondID)).name).toBe("");await plain.close();
    await ignore(secondID);
    const reader=await browser.newContext({httpCredentials:{username:"reader",password:"secret"}});
    expect((await reader.request.get(baseURL+"/photos/people?ignored=1&format=json")).ok()).toBe(true);
    const readerPage=await reader.newPage();await readerPage.goto(baseURL+"/photos/people?ignored=1");
    await expect(readerPage.locator("[data-ignored-face]")).toHaveCount(1);
    await expect(readerPage.locator("[data-ignored-edit], .ignored-face-form, [data-person-dialog]")).toHaveCount(0);
    await reader.close();expect(errors).toEqual([]);
  } finally {await context.close();}
});
