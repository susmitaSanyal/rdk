package segmentation

import (
	"testing"

	"go.viam.com/test"

	pc "go.viam.com/core/pointcloud"
)

func createPointClouds(t *testing.T) *Clusters {
	clouds := make([]pc.PointCloud, 0)
	cloudMap := make(map[pc.Vec3]int)
	for i := 0; i < 3; i++ {
		clouds = append(clouds, pc.New())
	}
	// create 1st cloud
	p00 := pc.NewBasicPoint(0, 0, 0)
	cloudMap[p00.Position()] = 0
	test.That(t, clouds[0].Set(p00), test.ShouldBeNil)
	p01 := pc.NewBasicPoint(0, 0, 1)
	cloudMap[p01.Position()] = 0
	test.That(t, clouds[0].Set(p01), test.ShouldBeNil)
	p02 := pc.NewBasicPoint(0, 1, 0)
	cloudMap[p02.Position()] = 0
	test.That(t, clouds[0].Set(p02), test.ShouldBeNil)
	p03 := pc.NewBasicPoint(0, 1, 1)
	cloudMap[p03.Position()] = 0
	test.That(t, clouds[0].Set(p03), test.ShouldBeNil)
	// create a 2nd cloud far away
	p10 := pc.NewBasicPoint(30, 0, 0)
	cloudMap[p10.Position()] = 1
	test.That(t, clouds[1].Set(p10), test.ShouldBeNil)
	p11 := pc.NewBasicPoint(30, 0, 1)
	cloudMap[p11.Position()] = 1
	test.That(t, clouds[1].Set(p11), test.ShouldBeNil)
	p12 := pc.NewBasicPoint(30, 1, 0)
	cloudMap[p12.Position()] = 1
	test.That(t, clouds[1].Set(p12), test.ShouldBeNil)
	p13 := pc.NewBasicPoint(30, 1, 1)
	cloudMap[p13.Position()] = 1
	test.That(t, clouds[1].Set(p13), test.ShouldBeNil)
	// create 3rd cloud
	p20 := pc.NewBasicPoint(0, 30, 0)
	cloudMap[p20.Position()] = 2
	test.That(t, clouds[2].Set(p20), test.ShouldBeNil)
	p21 := pc.NewBasicPoint(0, 30, 1)
	cloudMap[p21.Position()] = 2
	test.That(t, clouds[2].Set(p21), test.ShouldBeNil)
	p22 := pc.NewBasicPoint(1, 30, 0)
	cloudMap[p22.Position()] = 2
	test.That(t, clouds[2].Set(p22), test.ShouldBeNil)
	p23 := pc.NewBasicPoint(1, 30, 1)
	cloudMap[p23.Position()] = 2
	test.That(t, clouds[2].Set(p23), test.ShouldBeNil)
	p24 := pc.NewBasicPoint(0.5, 30, 0.5)
	cloudMap[p24.Position()] = 2
	test.That(t, clouds[2].Set(p24), test.ShouldBeNil)
	return &Clusters{clouds, cloudMap}
}

func TestAssignCluter(t *testing.T) {
	clusters := createPointClouds(t)
	test.That(t, clusters.N(), test.ShouldEqual, 3)
	// assign a new cluster
	p30 := pc.NewBasicPoint(30, 30, 1)
	test.That(t, clusters.AssignCluster(p30, 3), test.ShouldBeNil)
	test.That(t, clusters.N(), test.ShouldEqual, 4)
	test.That(t, clusters.Indices[p30.Position()], test.ShouldEqual, 3)
	// assign a new cluster with a large index
	pNew := pc.NewBasicPoint(30, 30, 30)
	test.That(t, clusters.AssignCluster(pNew, 100), test.ShouldBeNil)
	test.That(t, clusters.N(), test.ShouldEqual, 101)
	test.That(t, clusters.Indices[pNew.Position()], test.ShouldEqual, 100)
}

func TestMergeCluster(t *testing.T) {
	clusters := createPointClouds(t)
	// before merge
	test.That(t, clusters.PointClouds[0].Size(), test.ShouldEqual, 4)
	test.That(t, clusters.PointClouds[1].Size(), test.ShouldEqual, 4)
	test.That(t, clusters.PointClouds[2].Size(), test.ShouldEqual, 5)
	for i := 0; i < 2; i++ {
		clusters.PointClouds[i].Iterate(func(pt pc.Point) bool {
			test.That(t, clusters.Indices[pt.Position()], test.ShouldEqual, i)
			return true
		})
	}
	// merge
	test.That(t, clusters.MergeClusters(0, 1), test.ShouldBeNil)
	// after merge
	test.That(t, clusters.PointClouds[0].Size(), test.ShouldEqual, 0)
	test.That(t, clusters.PointClouds[1].Size(), test.ShouldEqual, 8)
	test.That(t, clusters.PointClouds[2].Size(), test.ShouldEqual, 5)
	for i := 0; i < 2; i++ {
		clusters.PointClouds[i].Iterate(func(pt pc.Point) bool {
			test.That(t, clusters.Indices[pt.Position()], test.ShouldEqual, 1)
			return true
		})
	}
	// merge to new cluster
	test.That(t, clusters.MergeClusters(2, 3), test.ShouldBeNil)
	// after merge
	test.That(t, clusters.PointClouds[0].Size(), test.ShouldEqual, 0)
	test.That(t, clusters.PointClouds[1].Size(), test.ShouldEqual, 8)
	test.That(t, clusters.PointClouds[2].Size(), test.ShouldEqual, 0)
	test.That(t, clusters.PointClouds[3].Size(), test.ShouldEqual, 5)
}
