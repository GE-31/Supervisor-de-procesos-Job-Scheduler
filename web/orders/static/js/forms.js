// Validación y envío del formulario de registro de pedidos. orders-api sigue
// siendo responsable de la validación de negocio y del cálculo real del
// total; aquí solo se ofrece una estimación y feedback inmediato al usuario.

const MAX_TEXT_LENGTH = 200;

function fieldError(name, message) {
  const node = document.querySelector(`#err-${name}`);
  if (node) node.textContent = message || "";
  const wrapper = document.querySelector(`#f-${name}`)?.closest(".field");
  if (wrapper) wrapper.classList.toggle("invalid", Boolean(message));
}

// validateForm revisa los campos obligatorios y devuelve { valid, values }.
// values solo se usa cuando valid es true.
export function validateForm(form) {
  const customer = form.customer.value.trim();
  const product = form.product.value.trim();
  const quantity = Number(form.quantity.value);
  const unitPrice = Number(form.unit_price.value);
  let valid = true;

  if (!customer) {
    fieldError("customer", "El cliente es obligatorio");
    valid = false;
  } else if (customer.length > MAX_TEXT_LENGTH) {
    fieldError("customer", `Máximo ${MAX_TEXT_LENGTH} caracteres`);
    valid = false;
  } else {
    fieldError("customer", "");
  }

  if (!product) {
    fieldError("product", "El producto es obligatorio");
    valid = false;
  } else if (product.length > MAX_TEXT_LENGTH) {
    fieldError("product", `Máximo ${MAX_TEXT_LENGTH} caracteres`);
    valid = false;
  } else {
    fieldError("product", "");
  }

  if (!Number.isFinite(quantity) || quantity <= 0 || !Number.isInteger(quantity)) {
    fieldError("quantity", "La cantidad debe ser un entero mayor que cero");
    valid = false;
  } else {
    fieldError("quantity", "");
  }

  if (!Number.isFinite(unitPrice) || unitPrice <= 0) {
    fieldError("unit-price", "El precio debe ser mayor que cero");
    valid = false;
  } else {
    fieldError("unit-price", "");
  }

  return { valid, values: { customer, product, quantity, unit_price: unitPrice } };
}

export function updateEstimate(form) {
  const quantity = Number(form.quantity.value);
  const unitPrice = Number(form.unit_price.value);
  const total = Number.isFinite(quantity) && Number.isFinite(unitPrice) ? quantity * unitPrice : 0;
  const node = document.querySelector("#estimate-total");
  if (node) node.textContent = `$${total.toFixed(2)}`;
}

export function clearFieldErrors() {
  ["customer", "product", "quantity", "unit-price"].forEach((name) => fieldError(name, ""));
}

export function setFormBusy(form, busy) {
  const submitButton = document.querySelector("#btn-submit-form");
  [...form.elements].forEach((element) => {
    element.disabled = busy;
  });
  if (submitButton) submitButton.textContent = busy ? "Registrando…" : "Registrar pedido";
}
