// Checks that every function a template calls actually exists in its script.
//
// ESLint does not resolve template expressions against `<script setup>`
// bindings, so removing a helper that the markup still calls compiles, builds
// and ships. It fails at render time, and because Vue aborts the whole
// subtree, the symptom is a section of the page silently missing rather than
// an error anyone connects to the edit that caused it.

import { readFileSync, readdirSync } from 'node:fs';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');

// Anything the language or the framework provides, plus the members that show
// up as `something.method(` after the object part is stripped.
const AMBIENT = new Set([
  't', 'te', 'tm', 'd', 'n', 'rt', '$t', 'emit', 'slots', 'attrs',
  'Math', 'Object', 'Array', 'String', 'Number', 'Boolean', 'JSON', 'Date',
  'Set', 'Map', 'RegExp', 'parseInt', 'parseFloat', 'isNaN', 'encodeURIComponent',
  'decodeURIComponent', 'console', 'window', 'document', 'require',
  'if', 'for', 'while', 'switch', 'catch', 'return', 'typeof', 'in', 'of', 'new',
]);

// Walk src/ rather than glob: node's globSync is not available on every
// version this project is built with.
const listVueFiles = (dir) => {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...listVueFiles(path));
    else if (entry.name.endsWith('.vue')) out.push(path);
  }
  return out;
};

const files = listVueFiles(join(root, 'src'));
let failures = 0;

for (const file of files) {
  const source = readFileSync(file, 'utf8');

  const scriptStart = source.indexOf('<script setup>');
  const templateStart = source.indexOf('<template>');
  if (scriptStart === -1 || templateStart === -1) continue; // options API or no markup

  const script = source.slice(scriptStart, source.indexOf('</script>', scriptStart));
  const template = source.slice(templateStart, source.lastIndexOf('</template>'));

  const defined = new Set(AMBIENT);
  for (const match of script.matchAll(/\b(?:const|let|var|function)\s+([A-Za-z_$][\w$]*)/g)) {
    defined.add(match[1]);
  }
  // Destructured bindings: const { a, b: c } = useThing()
  for (const block of script.matchAll(/\b(?:const|let|var)\s*[{[]([^}\]]*)[}\]]\s*=/g)) {
    for (const name of block[1].split(',')) {
      const bound = name.includes(':') ? name.split(':').pop() : name;
      const clean = bound.replace(/[^\w$]/g, '');
      if (clean) defined.add(clean);
    }
  }
  for (const match of script.matchAll(/\bimport\s+([A-Za-z_$][\w$]*)\s+from/g)) {
    defined.add(match[1]);
  }
  for (const block of script.matchAll(/import\s*{([^}]*)}\s*from/g)) {
    for (const name of block[1].split(',')) {
      if (name.trim()) defined.add(name.trim().split(/\s+as\s+/).pop());
    }
  }
  // defineProps({ mailboxes: ... }) exposes each key to the template.
  for (const block of script.matchAll(/defineProps\s*\(\s*{([\s\S]*?)}\s*\)/g)) {
    for (const key of block[1].matchAll(/^\s*([A-Za-z_$][\w$]*)\s*:/gm)) defined.add(key[1]);
  }
  // v-for and slot bindings introduce their own names.
  for (const match of template.matchAll(/v-for="\(?\s*([A-Za-z_$][\w$]*)/g)) defined.add(match[1]);
  for (const match of template.matchAll(/#\w+="{?\s*([A-Za-z_$][\w$]*)/g)) defined.add(match[1]);

  // Only look where Vue actually evaluates JavaScript: interpolations and
  // directive/bound attribute values. Prose and HTML comments are not code,
  // and a sentence like "<!-- Tabs (one per service) -->" is not a call.
  const expressions = [];
  const markup = template.replace(/<!--[\s\S]*?-->/g, '');
  for (const match of markup.matchAll(/{{([\s\S]*?)}}/g)) expressions.push(match[1]);
  for (const match of markup.matchAll(/(?:^|\s)(?:v-[\w:.-]+|[:@#][\w:.-]+)="([^"]*)"/g)) {
    expressions.push(match[1]);
  }

  // A bare `name(` — not `obj.name(`, which is a method on some value.
  const missing = new Set();
  for (const expression of expressions) {
    // Strip string literals first: a bound style holding `calc(100vh - 2rem)`
    // is CSS text, not a call into the component.
    const code = expression
      .replace(/`[^`]*`/g, '``')
      .replace(/'[^']*'/g, "''")
      .replace(/\\"[^\\"]*\\"/g, '""');

    for (const call of code.matchAll(/(^|[^.\w$])([a-zA-Z_$][\w$]*)\s*\(/g)) {
      if (!defined.has(call[2])) missing.add(call[2]);
    }
  }

  if (missing.size) {
    failures++;
    console.error(`✖ ${relative(root, file)}`);
    for (const name of missing) console.error(`    ${name}() is called in the template but not defined`);
  }
}

if (failures) {
  console.error(`\n${failures} component(s) call something their script does not define.`);
  process.exit(1);
}

console.log(`All ${files.length} component templates resolve.`);
