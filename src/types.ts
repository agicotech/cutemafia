export type Price = { label: string; value: string }

export type MediaAsset = {
  url: string
  mediaType: 'image' | 'video'
  name?: string
}

export type Kitten = {
  id: string
  name: string
  birthDate: string
  color: string
  breedClass: string
  generation: string
  status: string
  description: string
  mainPhoto: string
  featuredVideo: string | null
  media: MediaAsset[]
  prices: Price[]
}

export type Inquiry = {
  name: string
  contact: string
  message: string
  kittenId: string | null
}
