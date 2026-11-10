// payments: charges orders. Reached only through the gateway at /payments/*.
import { createServer } from "node:http";

createServer((req, res) => {
  if (req.method === "POST" && req.url === "/charge") {
    res.end(JSON.stringify({ status: "charged" }));
    return;
  }
  res.statusCode = 404;
  res.end();
}).listen(8080);
