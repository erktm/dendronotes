// Drawings ("*.drawing.svg") and the icon library of a soma.

import { request } from './client'
import { encodePath } from '../paths'
import { somaBase } from './base'

export interface DrawingFile {
  path: string
  /** Only when reading. */
  svg?: string
  etag: string
}

const url = (path: string) => `${somaBase()}/drawings/${encodePath(path)}`

export const readDrawing = (path: string) => request<DrawingFile>('GET', url(path))

/** Creates an empty drawing; with unique, a taken name gets a " (n)" suffix. */
export const createDrawing = (path: string, unique = true) =>
  request<DrawingFile>('POST', unique ? `${url(path)}?unique=1` : url(path))

export const saveDrawing = (path: string, svg: string, etag: string) =>
  request<DrawingFile>('PUT', url(path), { body: { svg }, headers: { 'If-Match': `"${etag}"` } })

export interface IconInfo {
  id: number
  name: string
  mime: 'image/png' | 'image/svg+xml'
  size: number
  createdBy: number
  createdAt: string
}

export const icons = () => request<{ icons: IconInfo[] }>('GET', `${somaBase()}/icons`).then((r) => r.icons)

/** Adds a PNG (at most 512 px) or SVG to the library. */
export const addIcon = (name: string, picture: Blob) =>
  request<IconInfo>('POST', `${somaBase()}/icons?name=${encodeURIComponent(name)}`, { raw: picture })

export const deleteIcon = (id: number) => request<{ id: number }>('DELETE', `${somaBase()}/icons/${id}`)

export const iconURL = (id: number) => `${somaBase()}/icons/${id}`
