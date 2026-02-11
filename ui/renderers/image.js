export async function renderImages(ctx, frame) {
  for (const imgBlock of frame.images) {
    const img = new Image()
    img.src = imgBlock.path

    await new Promise(resolve => {
      img.onload = resolve
    })

    ctx.drawImage(
      img,
      40,
      120,
      imgBlock.width,
      imgBlock.height
    )
  }
}
