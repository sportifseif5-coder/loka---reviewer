import { cpSync, mkdirSync, rmSync } from "node:fs";

rmSync("dist", { recursive: true, force: true });
mkdirSync("dist", { recursive: true });

for (const file of ["index.html", "style.css", "main.js"]) {
  cpSync(file, `dist/${file}`);
}

console.log("frontend copied to dist/");
