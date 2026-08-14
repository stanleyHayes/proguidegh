import base from "@proguidegh/config/eslint.config.base.mjs";

export default [
  ...base,
  // The service worker runs in a ServiceWorkerGlobalScope, not Node/DOM.
  { ignores: ["public/sw.js"] },
];
