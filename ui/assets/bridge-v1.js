// Sub2API UI Bridge v1 客户端：校验来源窗口、消息标识、Bridge Token 与请求 ID。
(function () {
  "use strict";
  var HOST_SOURCE = "sub2api-plugin-host";
  var UI_SOURCE = "sub2api-plugin-ui";
  var TIMEOUT_MS = 35000;

  function readToken() {
    var hash = window.location.hash.replace(/^#/, "");
    var params = new URLSearchParams(hash);
    return params.get("bridge_token") || "";
  }

  function createBridge() {
    var token = readToken();
    var pending = new Map();
    var counter = 0;

    function onMessage(event) {
      if (event.source !== window.parent) return;
      var data = event.data;
      if (!data || data.source !== HOST_SOURCE || data.bridge_token !== token) return;
      var entry = pending.get(data.request_id);
      if (!entry) return;
      pending.delete(data.request_id);
      clearTimeout(entry.timer);
      entry.resolve(data);
    }
    window.addEventListener("message", onMessage);

    function post(message) {
      message.source = UI_SOURCE;
      message.bridge_token = token;
      window.parent.postMessage(message, "*");
    }

    function request(type, extra) {
      counter += 1;
      var id = type + "-" + Date.now().toString(36) + "-" + counter;
      return new Promise(function (resolve, reject) {
        var timer = setTimeout(function () {
          pending.delete(id);
          reject(new Error("宿主响应超时"));
        }, TIMEOUT_MS);
        pending.set(id, { resolve: resolve, reject: reject, timer: timer });
        var message = Object.assign({ type: type, request_id: id }, extra || {});
        post(message);
      });
    }

    return {
      hasToken: token !== "",
      ready: function () { post({ type: "sub2api.plugin.ready" }); },
      request: request,
      resize: function (height) { post({ type: "ui.resize", height: height }); },
      notify: function (level, message) { post({ type: "ui.notify", level: level, message: message }); },
      dispose: function () {
        window.removeEventListener("message", onMessage);
        pending.forEach(function (entry) {
          clearTimeout(entry.timer);
          entry.reject(new Error("页面已卸载"));
        });
        pending.clear();
      }
    };
  }

  window.Sub2APIBridge = { create: createBridge };
})();
