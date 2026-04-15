function renderTextFrame(ctx, canvas, frame) {
  // We don't clear here if we want to draw on top of images
  // The caller (renderCurrentView) clears the canvas.

  frame.texts.forEach(text => {
    ctx.fillStyle = text.color || "#ffffff"
    ctx.font = `${text.size}px 'Space Mono', monospace`
    
    // Set alignment to center horizontally
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    
    // Use the original Y position to prevent overlap, 
    // but force X to center (120)
    // Add a small padding to the bottom of each text line (e.g. + 5px)
    ctx.fillText(text.text, 120, text.y + 5)
  })
}
