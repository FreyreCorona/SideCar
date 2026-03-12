async function renderImages(ctx, frame) {
  for (const imgBlock of frame.images) {
    const img = new Image()
    img.src = imgBlock.path

    await new Promise(resolve => {
      img.onload = resolve
      img.onerror = resolve // avoid hanging
    })

    ctx.drawImage(
      img,
      imgBlock.x,
      imgBlock.y,
      imgBlock.width,
      imgBlock.height
    )
  }
}
