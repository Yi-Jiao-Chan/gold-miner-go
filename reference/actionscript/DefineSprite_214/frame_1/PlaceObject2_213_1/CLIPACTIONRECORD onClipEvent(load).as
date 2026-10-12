onClipEvent(load){
   object = 12;
   if(eval("/:cloverBonus") == 1)
   {
      w2 = random(6) + 1;
      if(2 >= w2)
      {
         price = "power";
      }
      else if(4 >= w2)
      {
         if(3 >= eval("/:stick"))
         {
            price = "dynamite";
         }
         else
         {
            price = random(300) + 300;
         }
      }
      else
      {
         price = 700;
      }
   }
   else
   {
      w2 = random(6) + 1;
      if(3 >= w2)
      {
         price = random(600) + 1;
      }
      else if(w2 == 4)
      {
         price = "power";
      }
      else if(w2 == 5)
      {
         if(3 >= eval("/:stick"))
         {
            price = "dynamite";
         }
         else
         {
            price = random(100) + 100;
         }
      }
      else
      {
         price = 800;
      }
   }
   w = random(4) + 1;
   if(w == 1)
   {
      weight = random(9) + 1;
   }
   else if(w == 2)
   {
      weight = (random(5) + 1) * -1;
   }
   else if(w == 3)
   {
      weight = 9;
   }
   else if(w >= 4)
   {
      weight = -1;
   }
}
