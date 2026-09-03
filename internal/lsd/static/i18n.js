/* Translation for the lsd UI. Keys are the English strings; a dictionary
 * per language maps them to translations. Static markup is translated by
 * walking [data-i18n] (text), [data-i18n-html] (inner markup with inline
 * tags) and [data-i18n-attr] (a space-separated list of attributes).
 * Language: the lsd-lang override in localStorage, else the scooter's
 * dashboard.language setting once known, else the browser. */
"use strict";

const I18N = {
  dicts: {},
  lang: "en",
  available: ["en", "de"],
  pick(fromScooter) {
    const override = localStorage.getItem("lsd-lang");
    const wanted = override || fromScooter || navigator.language || "en";
    const short = String(wanted).slice(0, 2).toLowerCase();
    return this.available.includes(short) ? short : "en";
  },
  t(key, vars) {
    const d = this.dicts[this.lang] || {};
    let s = Object.prototype.hasOwnProperty.call(d, key) ? d[key] : key;
    if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(v);
    return s;
  },
  // Translate the static markup. Originals are remembered on first pass so a
  // later language switch translates from English, not from German.
  apply(root = document) {
    for (const el of root.querySelectorAll("[data-i18n]")) {
      // The key is the element's own text, ignoring nested elements (icons, hold tips).
      const textNodes = [...el.childNodes].filter(n => n.nodeType === 3 && n.textContent.trim());
      if (!el.dataset.i18nKey) el.dataset.i18nKey = textNodes.map(n => n.textContent).join(" ").replace(/\s+/g, " ").trim();
      const tr = this.t(el.dataset.i18nKey);
      if (textNodes.length === 1) textNodes[0].textContent = textNodes[0].textContent.replace(textNodes[0].textContent.trim(), tr);
      else if (textNodes.length === 0 && !el.children.length) el.textContent = tr;
    }
    for (const el of root.querySelectorAll("[data-i18n-html]")) {
      if (!el.dataset.i18nKey) el.dataset.i18nKey = el.innerHTML.trim().replace(/\s+/g, " ");
      el.innerHTML = this.t(el.dataset.i18nKey);
    }
    for (const el of root.querySelectorAll("[data-i18n-attr]")) {
      for (const attr of el.dataset.i18nAttr.split(/\s+/)) {
        const store = `data-i18n-src-${attr}`;
        if (!el.hasAttribute(store)) el.setAttribute(store, el.getAttribute(attr) || "");
        el.setAttribute(attr, this.t(el.getAttribute(store)));
      }
    }
    document.documentElement.lang = this.lang;
  },
};
const t = (key, vars) => I18N.t(key, vars);
