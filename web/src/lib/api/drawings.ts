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
  /** Left out for icons without a section. */
  sectionId?: number
  createdBy: number
  createdAt: string
}

export interface IconSection {
  id: number
  name: string
  position: number
}

export interface IconLibrary {
  icons: IconInfo[]
  sections: IconSection[]
}

export interface PackResult {
  added: number
  sections: string[]
  skipped: { file: string; reason: string }[]
}

/** The soma's icon library and the one admins fill for everyone. */
export const icons = () => request<IconLibrary & { server: IconLibrary }>('GET', `${somaBase()}/icons`)

/** The server's library as admins manage it. */
export const serverLibrary = () => request<IconLibrary>('GET', '/api/admin/icons')

/** What can be done with an icon library: a soma's, or the server's (admins). */
export interface LibraryApi {
  /** Adds a PNG (at most 512 px) or SVG, in a section or none (0). */
  add(name: string, picture: Blob, section?: number): Promise<IconInfo>
  /** Adds a ZIP of pictures; its folders become sections. */
  addPack(file: File): Promise<PackResult>
  move(id: number, section: number): Promise<IconInfo>
  remove(id: number): Promise<unknown>
  addSection(name: string): Promise<IconSection>
  renameSection(id: number, name: string): Promise<unknown>
  orderSections(ids: number[]): Promise<unknown>
  removeSection(id: number): Promise<{ icons: number }>
}

function libraryApi(base: () => string): LibraryApi {
  return {
    add: (name, picture, section = 0) =>
      request<IconInfo>('POST', `${base()}/icons?name=${encodeURIComponent(name)}${section ? `&section=${section}` : ''}`, { raw: picture }),
    addPack: (file) => request<PackResult>('POST', `${base()}/icons/pack?name=${encodeURIComponent(file.name)}`, { raw: file }),
    move: (id, section) => request<IconInfo>('PATCH', `${base()}/icons/${id}`, { body: { sectionId: section } }),
    remove: (id) => request('DELETE', `${base()}/icons/${id}`),
    addSection: (name) => request<IconSection>('POST', `${base()}/icon-sections`, { body: { name } }),
    renameSection: (id, name) => request('PATCH', `${base()}/icon-sections/${id}`, { body: { name } }),
    orderSections: (ids) => request('PUT', `${base()}/icon-sections/order`, { body: { ids } }),
    removeSection: (id) => request<{ icons: number }>('DELETE', `${base()}/icon-sections/${id}`),
  }
}

export const somaIcons = libraryApi(() => somaBase())
export const serverIcons = libraryApi(() => '/api/admin')

/** The picture of a soma icon, and of a server icon. */
export const iconURL = (id: number) => `${somaBase()}/icons/${id}`
export const serverIconURL = (id: number) => `/api/icons/${id}`
