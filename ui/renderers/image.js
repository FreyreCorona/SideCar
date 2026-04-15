async function renderImages(ctx, frame) {
  for (const imgBlock of frame.images) {
    const img = new Image()
    img.src = imgBlock.path

    await new Promise(resolve => {
      img.onload = resolve
      img.onerror = resolve // avoid hanging
    })

    // Centered in the 240x240 canvas
    const x = 120 - (imgBlock.width / 2);
    const y = 120 - (imgBlock.height / 2);

    ctx.drawImage(
      img,
      x,
      y,
      imgBlock.width,
      imgBlock.height
    )
  }
}
