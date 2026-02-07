
const state = {
  assets: [],
  selected: null
};

const canvas = document.getElementById("previewCanvas");
const ctx = canvas.getContext("2d");

function resizeCanvas() {
  canvas.width = canvas.clientWidth;
  canvas.height = canvas.clientHeight;
  renderPreview();
}

window.addEventListener("resize", resizeCanvas);

function loadAssets() {
  // esta lista vendrá de Go más adelante si quieres
  state.assets = [
    { id: "screensaver1", src: "statics/screensaver1.gif" },
    { id: "screensaver2", src: "statics/screensaver2.gif" }
  ];

  const list = document.getElementById("assetList");
  list.innerHTML = "";

  state.assets.forEach(asset => {
    const li = document.createElement("li");
    li.textContent = asset.id;
    li.onclick = () => selectAsset(asset);
    list.appendChild(li);
  });
}

function selectAsset(asset) {
  state.selected = asset;
  document.querySelectorAll("#assetList li")
    .forEach(li => li.classList.remove("selected"));

  event.target.classList.add("selected");
  renderPreview();
}

function renderPreview() {
  if (!state.selected) {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    return;
  }

  const img = new Image();
  img.onload = () => {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
  };
  img.src = state.selected.src;
}

document.getElementById("setBtn").onclick = () => {
  if (!state.selected) return;

  // aquí llamas a Go
  window.setImage(JSON.stringify({
    asset: state.selected.id
  }));
};

resizeCanvas();
loadAssets();
