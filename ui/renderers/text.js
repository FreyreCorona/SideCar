function renderTextFrame(ctx, canvas, frame) {
  ctx.clearRect(0, 0, canvas.width, canvas.height)
  ctx.fillStyle = "#ffffff"
  ctx.textBaseline = "top"

  let y = 40

  frame.texts.forEach(text => {
    ctx.font = `${text.size}px system-ui`
    ctx.fillText(text.text, 40, y)
    y += text.size + 20
  })
}
