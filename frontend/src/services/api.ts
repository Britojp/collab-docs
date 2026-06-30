const BASE = '/api'

function authHeaders(): HeadersInit {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...authHeaders(), ...init?.headers },
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? err.message ?? 'request failed')
  }
  return res.json()
}

export interface AuthResponse {
  token: string
  userId: string
  name: string
  email: string
}

export interface DocumentSummary {
  id: string
  title: string
  ownerId: string
  updatedAt: string
}

export interface DocumentDetail extends DocumentSummary {
  content: string
  version: number
  createdAt: string
}

export interface Metrics {
  docId: string
  totalOps: number
  charsInserted: number
  charsDeleted: number
  lastActivity: string
}

export interface DocumentAnalytics {
  docId: string
  charCount: number
  wordCount: number
  lineCount: number
  paragraphCount: number
  version: number
  lastActivity: string
}

export const api = {
  login(email: string, password: string) {
    return request<AuthResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    })
  },
  register(email: string, name: string, password: string) {
    return request<AuthResponse>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, name, password }),
    })
  },
  listDocuments() {
    return request<DocumentSummary[]>('/documents')
  },
  getDocument(id: string) {
    return request<DocumentDetail>(`/documents/${id}`)
  },
  createDocument(title: string) {
    return request<DocumentSummary>('/documents', {
      method: 'POST',
      body: JSON.stringify({ title }),
    })
  },
  async deleteDocument(id: string) {
    const res = await fetch(`${BASE}/documents/${id}`, {
      method: 'DELETE',
      headers: authHeaders() as Record<string, string>,
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      throw new Error(err.error ?? err.message ?? 'delete failed')
    }
  },
  getMetrics(docId: string) {
    return request<Metrics>(`/metrics/${docId}`)
  },
  getDocumentAnalytics(docId: string) {
    return grpcWebGetDocumentAnalytics(docId)
  },
}

export function wsUrl(docId: string): string {
  const token = localStorage.getItem('token') ?? ''
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${protocol}://${location.host}/api/ws/${docId}?token=${token}`
}

async function grpcWebGetDocumentAnalytics(docId: string): Promise<DocumentAnalytics> {
  const payload = encodeDocumentAnalyticsRequest(docId)
  const framed = frameGrpcWebMessage(payload)
  const res = await fetch('/grpc/analytics.AnalyticsService/GetDocumentAnalytics', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/grpc-web+proto',
      'Accept': 'application/grpc-web+proto',
      'X-Grpc-Web': '1',
    },
    body: new Blob([framed.buffer as ArrayBuffer], { type: 'application/grpc-web+proto' }),
  })

  if (!res.ok) {
    throw new Error(`gRPC-Web request failed: ${res.status}`)
  }

  const bytes = new Uint8Array(await res.arrayBuffer())
  const message = firstGrpcWebDataMessage(bytes)
  if (!message) {
    throw new Error('gRPC-Web response did not include a data frame')
  }
  return decodeDocumentAnalyticsResponse(message)
}

function encodeDocumentAnalyticsRequest(docId: string): Uint8Array {
  return concatBytes([
    encodeStringField(1, docId),
  ])
}

function decodeDocumentAnalyticsResponse(bytes: Uint8Array): DocumentAnalytics {
  const out: DocumentAnalytics = {
    docId: '',
    charCount: 0,
    wordCount: 0,
    lineCount: 0,
    paragraphCount: 0,
    version: 0,
    lastActivity: '',
  }

  let offset = 0
  while (offset < bytes.length) {
    const key = readVarint(bytes, offset)
    offset = key.offset
    const field = Number(key.value >> 3n)
    const wireType = Number(key.value & 7n)

    if (wireType === 0) {
      const value = readVarint(bytes, offset)
      offset = value.offset
      const n = Number(value.value)
      if (field === 2) out.charCount = n
      else if (field === 3) out.wordCount = n
      else if (field === 4) out.lineCount = n
      else if (field === 5) out.paragraphCount = n
      else if (field === 6) out.version = n
    } else if (wireType === 2) {
      const len = readVarint(bytes, offset)
      offset = len.offset
      const end = offset + Number(len.value)
      const text = new TextDecoder().decode(bytes.slice(offset, end))
      offset = end
      if (field === 1) out.docId = text
      else if (field === 7) out.lastActivity = text
    } else {
      throw new Error(`unsupported protobuf wire type: ${wireType}`)
    }
  }

  return out
}

function frameGrpcWebMessage(message: Uint8Array): Uint8Array {
  const out = new Uint8Array(message.length + 5)
  out[0] = 0
  out[1] = (message.length >>> 24) & 0xff
  out[2] = (message.length >>> 16) & 0xff
  out[3] = (message.length >>> 8) & 0xff
  out[4] = message.length & 0xff
  out.set(message, 5)
  return out
}

function firstGrpcWebDataMessage(bytes: Uint8Array): Uint8Array | null {
  let offset = 0
  while (offset + 5 <= bytes.length) {
    const flags = bytes[offset]
    const len =
      (bytes[offset + 1] << 24) |
      (bytes[offset + 2] << 16) |
      (bytes[offset + 3] << 8) |
      bytes[offset + 4]
    offset += 5
    const end = offset + len
    if (end > bytes.length) {
      throw new Error('truncated gRPC-Web frame')
    }
    if ((flags & 0x80) === 0) {
      return bytes.slice(offset, end)
    }
    offset = end
  }
  return null
}

function encodeStringField(field: number, value: string): Uint8Array {
  const encoded = new TextEncoder().encode(value)
  return concatBytes([
    encodeVarint(BigInt((field << 3) | 2)),
    encodeVarint(BigInt(encoded.length)),
    encoded,
  ])
}

function encodeVarint(value: bigint): Uint8Array {
  const out: number[] = []
  let v = value
  while (v >= 0x80n) {
    out.push(Number((v & 0x7fn) | 0x80n))
    v >>= 7n
  }
  out.push(Number(v))
  return new Uint8Array(out)
}

function readVarint(bytes: Uint8Array, offset: number): { value: bigint; offset: number } {
  let shift = 0n
  let value = 0n
  while (offset < bytes.length) {
    const b = bytes[offset++]
    value |= BigInt(b & 0x7f) << shift
    if ((b & 0x80) === 0) {
      return { value, offset }
    }
    shift += 7n
  }
  throw new Error('unterminated protobuf varint')
}

function concatBytes(chunks: Uint8Array[]): Uint8Array {
  const total = chunks.reduce((sum, chunk) => sum + chunk.length, 0)
  const out = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    out.set(chunk, offset)
    offset += chunk.length
  }
  return out
}
