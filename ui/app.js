console.log("App loaded")

const state = {
  assets: [],
  selected: null,
  image: new Image()
};

const canvas = document.getElementById("previewCanvas");
const ctx = canvas ? canvas.getContext("2d") : null;
const assetList = document.getElementById("assetList");
const setBtn = document.getElementById("setBtn");
const imageInput = document.getElementById("imageInput");

function resizeCanvas() {
  const { clientWidth, clientHeight } = canvas;
  canvas.width = clientWidth;
  canvas.height = clientHeight;
  renderPreview();
}

window.addEventListener("resize", resizeCanvas);
window.addEventListener("keydown", async (e) => {
  if (e.key === "ArrowRight") {
    await window.nextView()
    await renderCurrentView()
  }
})
window.onViewChanged = async function () {
  await renderCurrentView()
}

function selectAsset(asset, element) {
  state.selected = asset;
  setBtn.disabled = false;

  document.querySelectorAll(".asset-list li")
    .forEach(li => li.classList.remove("selected"));

  element.classList.add("selected");

  state.image.src = asset.src;
}

state.image.onload = renderPreview;

function renderPreview() {
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  if (!state.selected) return;

  ctx.drawImage(
    state.image,
    0,
    0,
    canvas.width,
    canvas.height
  );
}

async function renderCurrentView() {
  const frame = await window.getCurrentFrame()

  ctx.clearRect(0, 0, canvas.width, canvas.height)

  if (frame.texts) {
    renderTextFrame(ctx, canvas, frame)
  }

  if (frame.images) {
    await renderImages(ctx, frame)
  }
}

setBtn.addEventListener("click", () => {
  if (!state.selected) return;

  window.setImage(JSON.stringify({
    asset: state.selected.id
  }));
});

assetList.addEventListener("click", async (e) => {
  if (e.target.tagName !== "LI") return;

  document.querySelectorAll(".asset-list li")
    .forEach(li => li.classList.remove("selected"));
  e.target.classList.add("selected");

  const view = e.target.dataset.view;
  if (view === "image") {
    imageInput.click();
  } else {
    await renderCurrentView();
  }
});

imageInput.addEventListener("change", async (e) => {
  const file = e.target.files[0];
  if (!file) return;

  // Enviar imagen al backend
  const formData = new FormData();
  formData.append("image", file);

  const res = await fetch("/upload", {
    method: "POST",
    body: formData
  });

  const { path } = await res.json();

  // Renderizar la imagen en el canvas
  const img = new window.Image();
  img.onload = () => {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
  };
  img.src = path;
});

resizeCanvas();
renderCurrentView();