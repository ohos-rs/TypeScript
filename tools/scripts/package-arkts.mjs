import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
    copyFileSync,
    existsSync,
    mkdirSync,
    readFileSync,
    writeFileSync,
} from "node:fs";
import {
    dirname,
    join,
    resolve,
} from "node:path";
import { fileURLToPath } from "node:url";

// Compiler release tooling, not an arkdown application entry point. Never
// overwrite a previously published revision or mislabel a dirty build as HEAD.
const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const git = (...args) => execFileSync("git", args, { cwd: root });
const sha256 = data => createHash("sha256").update(data).digest("hex");
const commit = git("rev-parse", "HEAD").toString().trim();
const diff = git("diff", "--binary", "HEAD");
const added = git("ls-files", "--others", "--exclude-standard", "-z").toString().split("\0").filter(Boolean).sort();
const additions = added.map(name => ({ name, content: readFileSync(join(root, name)) }));
const sourceHash = createHash("sha256").update(commit).update(diff);
for (const { name, content } of additions) {
    sourceHash.update(JSON.stringify([name, content.length])).update(content);
}
const fingerprint = sourceHash.digest("hex");
const dirty = diff.length !== 0 || added.length !== 0;
const id = dirty ? `${commit.slice(0, 10)}-worktree-${fingerprint.slice(0, 12)}` : commit.slice(0, 10);
const output = join(root, "built/arkts", id);
if (existsSync(output)) throw new Error(`Refusing to overwrite ${output}`);
mkdirSync(output, { recursive: true });
writeFileSync(join(output, "SOURCE.patch"), diff);
for (const { name, content } of additions) {
    const path = join(output, "source-added", name);
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, content);
}
const targets = [
    { name: "arm_darwin", os: "darwin", arch: "arm64" },
    { name: "x64_linux_gnu", os: "linux", arch: "amd64" },
    { name: "x64_window_msvc", os: "windows", arch: "amd64" },
];
const checksums = [];
const go = execFileSync("go", ["version"], { cwd: root, encoding: "utf8" }).trim();
const regexpModule = JSON.parse(execFileSync("go", ["list", "-m", "-json", "github.com/dlclark/regexp2/v2"], {
    cwd: join(root, "tsc"),
    encoding: "utf8",
}));
for (const target of targets) {
    const directory = join(output, target.name);
    mkdirSync(directory);
    const binary = target.os === "windows" ? "tsc.exe" : "tsc";
    const args = ["build", "-p", "2", "-trimpath", "-ldflags=-s -w", "-o", join(directory, binary), "./tsc/cmd/tsc"];
    console.log(`Building ${target.name}`);
    execFileSync("go", args, {
        cwd: root,
        stdio: "inherit",
        env: { ...process.env, CGO_ENABLED: "0", GOOS: target.os, GOARCH: target.arch },
    });
    copyFileSync(join(root, "LICENSE.txt"), join(directory, "LICENSE.txt"));
    copyFileSync(join(root, "NOTICE.txt"), join(directory, "NOTICE.txt"));
    copyFileSync(join(root, "docs/arkts.md"), join(directory, "ARKTS.md"));
    const regexpNotices = join(directory, "licenses/regexp2");
    mkdirSync(regexpNotices, { recursive: true });
    for (const name of ["LICENSE", "ATTRIB"]) {
        copyFileSync(join(regexpModule.Dir, name), join(regexpNotices, name));
    }
    const metadata = {
        commit,
        dirty,
        source_fingerprint: fingerprint,
        go,
        GOOS: target.os,
        GOARCH: target.arch,
        CGO_ENABLED: "0",
        embedded_typescript_libs: true,
        binary_sha256: sha256(readFileSync(join(directory, binary))),
        validation: "Run frontend, API, corpus and JS CLI acceptance separately; cross-build success is not runtime validation.",
    };
    writeFileSync(join(directory, "BUILD.json"), `${JSON.stringify(metadata, null, 2)}\n`);
    const archive = target.name + (target.os === "windows" ? ".zip" : ".tar.gz");
    if (target.os === "windows") {
        execFileSync("zip", ["-q", "-r", archive, target.name], { cwd: output, stdio: "inherit" });
    }
    else {
        execFileSync("tar", ["-czf", archive, target.name], { cwd: output, stdio: "inherit" });
    }
    checksums.push(`${sha256(readFileSync(join(output, archive)))}  ${archive}`);
}
writeFileSync(join(output, "SHA256SUMS"), `${checksums.join("\n")}\n`);
console.log(output);
