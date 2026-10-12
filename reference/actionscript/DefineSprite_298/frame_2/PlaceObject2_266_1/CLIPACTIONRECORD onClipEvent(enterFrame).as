onClipEvent(enterFrame){
   x = 1;
   while(eval("/:things") >= x)
   {
      if(this.hitTest("_root.T" + x + ".ob.box") and _parent.busy == 0)
      {
         _parent.busy = 1;
         _parent.price = eval("_root.T" + x + ".ob.price");
         _parent.object = eval("_root.T" + x + ".ob.object");
         _parent.weight = eval("_root.T" + x + ".ob.weight");
         _parent.claw.gotoAndStop(_parent.object);
         _root.miner.gotoAndStop("up");
         eval("_root.T" + x).gotoAndStop(2);
         _parent.gotoAndPlay("up" + _parent.up);
      }
      x++;
   }
}
