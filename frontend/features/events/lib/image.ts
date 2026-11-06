/**
 * EVENTHUB-264: the event image. The browser reads the picked file as a data URL
 * string, which the server action decodes again and sends to the API as raw
 * bytes. The rules mirror the backend's PUT /events/{id}/image so a problem
 * shows up in the form instead of as a failed request.
 */

/** Same limit as the backend's MaxEventImageBytes. */
export const MAX_IMAGE_BYTES = 2 * 1024 * 1024

export const IMAGE_CONTENT_TYPES = ["image/png", "image/jpeg"] as const

export type ImageContentType = (typeof IMAGE_CONTENT_TYPES)[number]

/** The backend checks the bytes against their declared type, so do we. */
const MAGIC_BYTES: Record<ImageContentType, number[]> = {
  "image/png": [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a],
  "image/jpeg": [0xff, 0xd8, 0xff],
}

/**
 * Set on the edit page when a new draft was created but its image could not be
 * stored, so the organizer knows to pick it again.
 */
export const IMAGE_FAILED_PARAM = "bildFehler"

export const IMAGE_TOO_LARGE = "Das Bild darf höchstens 2 MB groß sein."
export const IMAGE_WRONG_TYPE = "Bitte wähle ein PNG- oder JPEG-Bild."

function isImageContentType(type: string): type is ImageContentType {
  return (IMAGE_CONTENT_TYPES as readonly string[]).includes(type)
}

/** Checks a picked file before it is read, returns an error message or nothing. */
export function validateImageFile(file: File): string | undefined {
  if (!isImageContentType(file.type)) {
    return IMAGE_WRONG_TYPE
  }

  if (file.size > MAX_IMAGE_BYTES) {
    return IMAGE_TOO_LARGE
  }

  return undefined
}

export interface DecodedImage {
  contentType: ImageContentType
  bytes: Uint8Array<ArrayBuffer>
}

/**
 * Turns the data URL from the form back into the raw image. The string comes
 * from the client, so type, size and content are all checked again.
 */
export function decodeImageDataUrl(
  dataUrl: string
): DecodedImage | { error: string } {
  const match = /^data:([^;,]+);base64,([A-Za-z0-9+/=]*)$/.exec(dataUrl)

  if (!match || !isImageContentType(match[1])) {
    return { error: IMAGE_WRONG_TYPE }
  }

  const contentType = match[1]
  const bytes = Uint8Array.from(Buffer.from(match[2], "base64"))

  if (bytes.length === 0) {
    return { error: IMAGE_WRONG_TYPE }
  }

  if (bytes.length > MAX_IMAGE_BYTES) {
    return { error: IMAGE_TOO_LARGE }
  }

  const magic = MAGIC_BYTES[contentType]

  if (!magic.every((byte, index) => bytes[index] === byte)) {
    return { error: IMAGE_WRONG_TYPE }
  }

  return { contentType, bytes }
}
