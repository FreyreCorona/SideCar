export function renderTextFrame(ctx, canvas, frame) {
  ctx.clearRect(0, 0, canvas.width, canvas.height)
  ctx.fillStyle = "#ffffff"
  ctx.textBaseline = "top"

  let y = 40

  frame.blocks.forEach(block => {
    ctx.font = `${block.size}px system-ui`
    ctx.fillText(block.text, 40, y)
    y += block.size + 20
  })
}
