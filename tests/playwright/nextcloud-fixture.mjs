// Isolated TLS protocol fixture. Never connects to an external Nextcloud account.
import https from "node:https";
import { execFileSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import path from "node:path";

export async function nextcloudFixture(root) {
  const cert = path.join(root, "nextcloud.crt"),
    key = path.join(root, "nextcloud.key");
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-keyout",
      key,
      "-out",
      cert,
      "-days",
      "1",
      "-subj",
      "/CN=localhost",
      "-addext",
      "subjectAltName=IP:127.0.0.1,DNS:localhost",
    ],
    { stdio: "ignore" },
  );
  const files = new Map(),
    directories = new Set(),
    log = [],
    logins = new Map();
  let baseURL;
  const xml = (text) =>
    String(text).replace(
      /[<>&"']/g,
      (ch) =>
        ({
          "<": "&lt;",
          ">": "&gt;",
          "&": "&amp;",
          '"': "&quot;",
          "'": "&apos;",
        })[ch],
    );
  const server = https.createServer(
    { key: await readFile(key), cert: await readFile(cert) },
    async (req, res) => {
      const url = new URL(req.url, baseURL),
        pathname = decodeURIComponent(url.pathname).replace(/\/$/, "");
      log.push({ method: req.method, path: pathname, headers: req.headers });
      const json = (data) => {
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify(data));
      };
      const code = (status) => {
        res.statusCode = status;
        res.end();
      };
      if (req.method === "POST" && pathname === "/index.php/login/v2") {
        const token = "token-" + (logins.size + 1),
          user = "user-" + (logins.size + 1);
        logins.set(token, { user, granted: false });
        directories.add(`/remote.php/dav/files/${user}`);
        json({
          login: baseURL + "/login/" + token,
          poll: { token, endpoint: baseURL + "/poll" },
        });
        return;
      }
      if (req.method === "GET" && pathname.startsWith("/login/")) {
        const token = pathname.slice(7);
        res.setHeader("Content-Type", "text/html");
        res.end(
          `<form action="/grant/${token}" method="post"><button>Zugriff erlauben</button></form>`,
        );
        return;
      }
      if (req.method === "POST" && pathname.startsWith("/grant/")) {
        const login = logins.get(pathname.slice(7));
        if (!login) return code(404);
        login.granted = true;
        res.end("Verbunden");
        return;
      }
      if (req.method === "POST" && pathname === "/poll") {
        let body = "";
        for await (const chunk of req) body += chunk;
        const login = logins.get(new URLSearchParams(body).get("token"));
        if (!login?.granted) return code(404);
        json({
          server: baseURL,
          loginName: login.user,
          appPassword: "test-only-" + login.user,
        });
        return;
      }
      const credentials = Buffer.from(
          (req.headers.authorization || "").slice(6),
          "base64",
        ).toString(),
        [user, password] = credentials.split(":");
      if (password !== "test-only-" + user) return code(401);
      if (pathname === "/ocs/v2.php/cloud/user")
        return json({ ocs: { data: { id: user } } });
      const userRoot = `/remote.php/dav/files/${user}`;
      if (pathname !== userRoot && !pathname.startsWith(userRoot + "/"))
        return code(403);
      if (req.method === "PROPFIND") {
        if (!directories.has(pathname) && !files.has(pathname))
          return code(404);
        const entry = (name) =>
          `<d:response><d:href>${xml(name.split("/").map(encodeURIComponent).join("/"))}</d:href><d:propstat><d:prop><d:getcontentlength>${files.get(name)?.length || 0}</d:getcontentlength><d:resourcetype>${directories.has(name) ? "<d:collection/>" : ""}</d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`;
        let entries = entry(pathname);
        if (req.headers.depth === "1")
          for (const name of [...directories, ...files.keys()])
            if (name !== pathname && path.posix.dirname(name) === pathname)
              entries += entry(name);
        res.writeHead(207, { "Content-Type": "application/xml" });
        res.end(`<d:multistatus xmlns:d="DAV:">${entries}</d:multistatus>`);
        return;
      }
      if (req.method === "MKCOL") {
        if (directories.has(pathname) || files.has(pathname)) return code(405);
        if (!directories.has(path.posix.dirname(pathname))) return code(409);
        directories.add(pathname);
        return code(201);
      }
      if (req.method === "PUT") {
        if (req.headers["if-none-match"] !== "*") return code(400);
        if (files.has(pathname) || directories.has(pathname)) return code(412);
        if (!directories.has(path.posix.dirname(pathname))) return code(409);
        const chunks = [];
        for await (const chunk of req) chunks.push(chunk);
        files.set(pathname, Buffer.concat(chunks));
        return code(201);
      }
      code(405);
    },
  );
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  baseURL = `https://127.0.0.1:${server.address().port}`;
  return {
    baseURL,
    cert,
    files,
    directories,
    log,
    close: async () => {
      server.closeAllConnections();
      await new Promise((resolve) => server.close(resolve));
    },
  };
}
