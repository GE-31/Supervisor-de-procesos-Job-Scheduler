// Toasts de éxito/error y confirmación de acciones destructivas mediante el
// <dialog> nativo de confirm-modal, para no depender de alert()/confirm().

const TOAST_DURATION_MS = 4000;

export function toast(message, type = "info") {
  const container = document.querySelector("#toasts");
  if (!container) return;
  const node = document.createElement("div");
  node.className = `toast${type === "success" ? " success" : ""}${type === "error" ? " error" : ""}`;
  node.setAttribute("role", type === "error" ? "alert" : "status");
  node.textContent = message;
  container.append(node);
  setTimeout(() => node.remove(), TOAST_DURATION_MS);
}

// confirmAction muestra el modal de confirmación y resuelve con true/false
// según el botón que el usuario pulse (o Escape, que cuenta como cancelar).
export function confirmAction(message) {
  const dialog = document.querySelector("#confirm-modal");
  const messageNode = document.querySelector("#confirm-message");
  messageNode.textContent = message;

  return new Promise((resolve) => {
    function onClose() {
      dialog.removeEventListener("close", onClose);
      resolve(dialog.returnValue === "confirm");
    }
    dialog.addEventListener("close", onClose);
    dialog.showModal();
  });
}
