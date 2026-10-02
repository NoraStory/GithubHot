// AnZhiYu 运行时极简适配层（改造版）：
// 提供主题 inline onclick 所依赖的 window.anzhiyu 方法（原版位于 /js/utils.js，已按 SPA 裁剪）。
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

  // 音乐开关（nav-music hoverTips / consoleMusic 用）
  anzhiyu.musicToggle = function () {
    var el = document.querySelector("#nav-music meting-js");
    var ap = el && el.aplayer;
    if (ap) ap.toggle();
  };
  anzhiyu.musicTelescopic = function () {
    var el = document.getElementById("nav-music");
    if (el) el.classList.toggle("stretch");
  };

  // 边栏显示控制
  anzhiyu.hideAsideBtn = function () {
    document.documentElement.classList.toggle("hide-aside");
  };

  // 快捷键 / 热评弹幕（改造版：仅提示，实际能力由本站功能替代）
  anzhiyu.keyboardToggle = function () { anzhiyu.snackbarShow("快捷键开关保留为占位", false, 2000); };
  anzhiyu.switchCommentBarrage = function () { anzhiyu.snackbarShow("热评弹幕暂未启用", false, 2000); };

  window.anzhiyu = anzhiyu;
})();
