// Checks that a routed page does not wrap itself in the layout the router
// already wraps it in.
//
// The router mounts every dashboard page as a child of LayoutAuthenticated. A
// page that also imports it renders the whole shell twice: two navbars, two
// sidebars, and its own content pushed somewhere nobody looks. Nothing errors —
// the page simply comes out wrong, which is hard to connect back to one import.

import { readFileSync } from 'node:fs';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const routerPath = join(root, 'src/router/index.js');
const router = readFileSync(routerPath, 'utf8');

// The views the router mounts under a layout: "component: () => import('@/views/X.vue')"
// inside the children of a route whose component is the layout itself.
const layoutName = 'LayoutAuthenticated';
const childBlock = router.slice(router.indexOf(`component: ${layoutName}`));

const routed = new Set();
for (const match of childBlock.matchAll(/import\('@\/views\/([^']+)'\)/g)) {
  routed.add(match[1]);
}
// Statically imported children, e.g. "component: Home".
for (const match of router.matchAll(/import\s+(\w+)\s+from\s+'@\/views\/([^']+)'/g)) {
  if (childBlock.includes(`component: ${match[1]}`)) routed.add(match[2]);
}

let failures = 0;
for (const view of routed) {
  let source;
  try {
    source = readFileSync(join(root, 'src/views', view), 'utf8');
  } catch {
    console.error(`✖ the router points at src/views/${view}, which does not exist`);
    failures++;
    continue;
  }

  if (source.includes(layoutName)) {
    console.error(`✖ ${relative(root, join('src/views', view))}`);
    console.error(`    wraps itself in ${layoutName}, which the router already provides`);
    failures++;
  }
}

if (failures) {
  console.error(`\n${failures} routed page(s) would render the layout twice.`);
  process.exit(1);
}

console.log(`All ${routed.size} routed pages use the layout the router gives them.`);
