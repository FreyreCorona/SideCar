function renderTextFrame(ctx, canvas, frame) {
  // We don't clear here if we want to draw on top of images
  // The caller (renderCurrentView) clears the canvas.

  frame.texts.forEach(text => {
    ctx.fillStyle = text.color || "#ffffff"
    ctx.font = `${text.size}px 'Space Mono', monospace`
    ctx.textBaseline = "top"
    
    // If X or Y are 0, we might want to keep the old stacking logic 
    // but for now let's use the absolute positions
    ctx.fillText(text.text, text.x, text.y)
  })
}
