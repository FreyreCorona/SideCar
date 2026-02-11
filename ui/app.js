import { renderTextFrame } from "./renderers/text.js"
import { renderImages } from "./renderers/image.js"

const state = {
  assets: [],
  selected: null,
  image: new Image()
};

const canvas = document.getElementById("previewCanvas");
const ctx = canvas.getContext("2d");
const assetList = document.getElementById("assetList");
const setBtn = document.getElementById("setBtn");

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




resizeCanvas();
renderCurrentView();