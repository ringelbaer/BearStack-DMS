package de.bearstack.people.data.remote

/** Normalized coordinates in the original after applying EXIF orientation. */
data class FaceBounds(val x: Float, val y: Float, val width: Float, val height: Float) {
    companion object {
        fun validated(x: Float, y: Float, width: Float, height: Float): FaceBounds? {
            if (!listOf(x,y,width,height).all { it.isFinite() } || width<=0 || height<=0) return null
            val left=x.coerceIn(0f,1f); val top=y.coerceIn(0f,1f)
            val right=(x+width).coerceIn(0f,1f); val bottom=(y+height).coerceIn(0f,1f)
            return if(right>left && bottom>top) FaceBounds(left,top,right-left,bottom-top) else null
        }
    }
}
