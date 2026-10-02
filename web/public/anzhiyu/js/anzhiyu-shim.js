// AnZhiYu 运行时极简适配层（改造版）：
// 提供主题 inline onclick 所依赖的 window.anzhiyu 方法（原版位于 /js/utils.js，已按 SPA 裁剪）。
// 状态类功能（弹幕/快捷键/边栏）通过 CustomEvent 通知 Vue 层，DOM 类同步在这里完成。
(function () {
  var anzhiyu = {};

  // 返回顶部（footer_mini_logo / nav-totop 用）
  anzhiyu.scrollToDest = function (pos) {
    pos = Number(pos) || 0;
    window.scrollTo({ top: pos, behavior: "smooth" });
  };

  // 轻量 snackbar（原版依赖 Snackbar 库，这里用同款小条实现）
  var snackTimer = null;
  anzhiyu.snackbarShow = function (text, _action, duration) {
    var old = document.querySelector(".anzhiyu-snackbar");
    if (old) old.remove();
    var bar = document.createElement("div");
    bar.className = "anzhiyu-snackbar";
    bar.textContent = text || "";
    document.body.appendChild(bar);
    clearTimeout(snackTimer);
    snackTimer = setTimeout(function () { bar.remove(); }, duration || 2000);
  };

  // 作者问候语随机切换（aside card-info sayhi）
  anzhiyu.changeSayHelloText = function () {
    var el = document.getElementById("author-info__sayhi");
    var skills = (window.GLOBAL_CONFIG && window.GLOBAL_CONFIG.authorStatus && window.GLOBAL_CONFIG.authorStatus.skills) || ["你好呀"];
    if (el) el.textContent = skills[Math.floor(Math.random() * skills.length)];
  };

  // 控制台关闭
  anzhiyu.hideConsole = function () {
    var c = document.getElementById("console");
    if (c) c.classList.remove("show");
  };

  // 音乐开关（nav-music hoverTips / consoleMusic / 右键菜单用）
  anzhiyu.musicToggle = function () {
    var el = document.querySelector("#nav-music meting-js");
    var ap = el && el.aplayer;
    if (ap) ap.toggle();
  };
  anzhiyu.musicSkipBack = function () {
    var el = document.querySelector("#nav-music meting-js");
    var ap = el && el.aplayer;
    if (ap) ap.skipBack();
  };
  anzhiyu.musicSkipForward = function () {
    var el = document.querySelector("#nav-music meting-js");
    var ap = el && el.aplayer;
    if (ap) ap.skipForward();
  };
  anzhiyu.musicTelescopic = function () {
    var el = document.getElementById("nav-music");
    if (el) el.classList.toggle("stretch");
  };
  anzhiyu.musicGetName = function () {
    var t = document.querySelector("#nav-music .aplayer-title");
    return t ? t.textContent : "";
  };

  // 边栏显示控制（html.hide-aside + 控制台按钮状态同步）
  anzhiyu.hideAsideBtn = function () {
    var on = document.documentElement.classList.toggle("hide-aside");
    var btn = document.getElementById("consoleHideAside");
    if (btn) on ? btn.classList.add("on") : btn.classList.remove("on");
    anzhiyu.snackbarShow(on ? "已切换单栏布局" : "已切换双栏布局", false, 1500);
  };

  // 热评弹幕开关：状态持久化 + 通知 Vue 层渲染/隐藏
  anzhiyu.switchCommentBarrage = function () {
    var current = localStorage.getItem("commentBarrageSwitch") !== "false";
    var next = !current;
    localStorage.setItem("commentBarrageSwitch", next ? "true" : "false");
    var btn = document.getElementById("consoleCommentBarrage");
    if (btn) next ? btn.classList.add("on") : btn.classList.remove("on");
    var menuText = document.querySelector(".menu-commentBarrage-text");
    if (menuText) menuText.textContent = next ? "关闭热评" : "显示热评";
    window.dispatchEvent(new CustomEvent("githubhot:barrage", { detail: { on: next } }));
    anzhiyu.snackbarShow(next ? "✨ 已开启热评弹幕" : "已关闭热评弹幕", false, 1500);
  };

  // 快捷键开关：状态持久化 + 通知 Vue 层
  anzhiyu.keyboardToggle = function () {
    var on = localStorage.getItem("keyboardToggle") !== "false";
    localStorage.setItem("keyboardToggle", on ? "false" : "true");
    var btn = document.getElementById("consoleKeyboard");
    if (btn) on ? btn.classList.remove("on") : btn.classList.add("on");
    window.dispatchEvent(new CustomEvent("githubhot:keyboard", { detail: { on: !on } }));
    anzhiyu.snackbarShow(!on ? "⌨️ 快捷键已开启" : "快捷键已关闭", false, 1500);
  };

  window.anzhiyu = anzhiyu;
})();
