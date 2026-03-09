// ../src/remote-chat-island.ts
(function() {
  const React = window.React;
  const ReactDOM = window.ReactDOM;
  if (!React || !ReactDOM || typeof ReactDOM.createRoot !== "function") {
    return;
  }
  const h = React.createElement;
  const roots = new WeakMap;
  function getRoot(container) {
    if (!container)
      return null;
    let root = roots.get(container);
    if (!root) {
      root = ReactDOM.createRoot(container);
      roots.set(container, root);
    }
    return root;
  }
  function senderLabel(role) {
    if (role === "user")
      return "You";
    if (role === "assistant")
      return "Agent";
    return "Carrier";
  }
  function ChatMessages(props) {
    const messages = Array.isArray(props.messages) ? props.messages : [];
    const children = messages.map((message, index) => {
      const role = String(message && message.role ? message.role : "system");
      const text = String(message && message.text ? message.text : "");
      const key = String(message && message.id ? message.id : "m-" + String(index));
      return h("div", { className: "chat-msg", key }, [
        h("span", { className: "sender", key: "sender" }, senderLabel(role) + ":"),
        h("span", { className: "body", key: "body" }, " " + text)
      ]);
    });
    return h(React.Fragment, null, children);
  }
  function renderMessages(container, messages) {
    const root = getRoot(container);
    if (!root)
      return false;
    root.render(h(ChatMessages, { messages }));
    window.requestAnimationFrame(() => {
      container.scrollTop = container.scrollHeight;
    });
    return true;
  }
  window.CarrierRemoteChatIsland = {
    renderMessages
  };
})();
