package de.bearstack.people

import androidx.compose.ui.geometry.Size
import de.bearstack.people.data.remote.FaceBounds
import de.bearstack.people.ui.originalTransform
import org.junit.Assert.*
import org.junit.Test

class OriginalTransformTest {
    @Test fun fitIncludesLetterboxingAndOrientedPortraitDimensions() {
        val box=FaceBounds(.1f,.2f,.3f,.4f)
        val landscape=originalTransform(Size(400f,600f),Size(2000f,1000f),box,0f)
        assertEquals(40f,landscape.face!!.left,.001f)
        assertEquals(240f,landscape.face.top,.001f)
        assertEquals(120f,landscape.face.width,.001f)
        assertEquals(80f,landscape.face.height,.001f)
        val portrait=originalTransform(Size(600f,400f),Size(1000f,2000f),box,0f)
        assertEquals(220f,portrait.face!!.left,.001f)
        assertEquals(80f,portrait.face.top,.001f)
        assertEquals(60f,portrait.face.width,.001f)
        assertEquals(160f,portrait.face.height,.001f)
    }
    @Test fun zoomMovesToSelectedFaceAndReversesWithoutDrift() {
        val viewport=Size(400f,600f);val image=Size(2000f,1000f);val box=FaceBounds(.7f,.4f,.1f,.2f)
        val start=originalTransform(viewport,image,box,0f)
        val middle=originalTransform(viewport,image,box,.5f)
        val end=originalTransform(viewport,image,box,1f)
        assertTrue(middle.scale>start.scale);assertTrue(end.scale>middle.scale)
        assertEquals(200f,end.face!!.center.x,.001f)
        assertEquals(300f,end.face.center.y,.001f)
        assertEquals(400f/1.5f,end.face.width,.001f)
        assertEquals(start,originalTransform(viewport,image,box,-1f))
        assertEquals(end,originalTransform(viewport,image,box,2f))
    }
    @Test fun edgeFacesStayVisibleAndTinyFacesHaveBoundedMagnification() {
        val viewport=Size(400f,600f)
        for(box in listOf(FaceBounds(0f,0f,.1f,.1f),FaceBounds(.999f,.999f,.001f,.001f))) {
            val transform=originalTransform(viewport,Size(4000f,2000f),box,1f)
            val rect=transform.face!!
            assertTrue(transform.scale<=12f)
            assertTrue(rect.left>=-.01f);assertTrue(rect.top>=-.01f)
            assertTrue(rect.right<=400.01f);assertTrue(rect.bottom<=600.01f)
        }
        assertNull(originalTransform(viewport,Size.Zero,FaceBounds(0f,0f,1f,1f),1f).face)
        assertNull(originalTransform(viewport,viewport,null,1f).face)
    }
    @Test fun invalidBoundsDoNotProduceInvalidTransforms() {
        assertNull(FaceBounds.validated(Float.NaN,0f,.1f,.1f))
        assertNull(FaceBounds.validated(0f,0f,Float.POSITIVE_INFINITY,.1f))
        assertNull(FaceBounds.validated(0f,0f,-1f,.1f))
        assertNull(FaceBounds.validated(2f,0f,.1f,.1f))
        assertEquals(FaceBounds(0f,0f,.5f,1f),FaceBounds.validated(-.1f,0f,.6f,2f))
    }
}
