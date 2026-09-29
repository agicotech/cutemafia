import type { Inquiry, Kitten, MediaAsset } from './types'

export const API_URL = import.meta.env.VITE_API_URL ?? 'http://127.0.0.1:8000'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    headers: init?.body instanceof FormData ? init.headers : { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const detail = await response.json().catch(() => null)
    throw new Error(detail?.detail ?? `API error: ${response.status}`)
  }
  return response.status === 204 ? undefined as T : response.json()
}

export const mediaUrl = (url: string) => url.startsWith('http') ? url : `${API_URL}${url}`
export const getKittens = () => request<Kitten[]>('/api/kittens')
const adminHeaders = (password: string) => ({ 'X-Admin-Password': password })
export const saveKitten = (kitten: Kitten, password: string) => request<Kitten>(`/api/kittens${kitten.id ? `/${kitten.id}` : ''}`, {
  method: kitten.id ? 'PUT' : 'POST',
  headers: adminHeaders(password),
  body: JSON.stringify(kitten),
})
export const deleteKitten = (id: string, password: string) => request<void>(`/api/kittens/${id}`, {
  method: 'DELETE',
  headers: adminHeaders(password),
})

export async function optimizeImage(file: File): Promise<File> {
  if (!file.type.startsWith('image/') || file.type === 'image/gif') return file
  try {
    const image = await createImageBitmap(file, { imageOrientation: 'from-image' })
    const scale = Math.min(1, 2560 / Math.max(image.width, image.height))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(image.width * scale))
    canvas.height = Math.max(1, Math.round(image.height * scale))
    const context = canvas.getContext('2d')
    if (!context) return file
    context.drawImage(image, 0, 0, canvas.width, canvas.height)
    image.close()
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/webp', .82))
    if (!blob || blob.size >= file.size) return file
    return new File([blob], file.name.replace(/\.[^.]+$/, '') + '.webp', { type: 'image/webp' })
  } catch {
    return file
  }
}

export const uploadMedia = async (file: File, password: string) => {
  const body = new FormData()
  body.append('file', file)
  return request<MediaAsset>('/api/uploads', { method: 'POST', headers: adminHeaders(password), body })
}
export const sendInquiry = (inquiry: Inquiry) => request<{ id: string; status: string }>('/api/inquiries', {
  method: 'POST',
  body: JSON.stringify(inquiry),
})
